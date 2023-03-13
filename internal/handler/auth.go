package handler

import (
	"fmt"
	"github.com/go-pg/pg/v10"
	"github.com/sakuraapp/gateway/internal/client"
	"github.com/sakuraapp/gateway/internal/gateway"
	"github.com/sakuraapp/shared/pkg/constant"
	dispatcher "github.com/sakuraapp/shared/pkg/dispatcher/gateway"
	"github.com/sakuraapp/shared/pkg/model"
	"github.com/sakuraapp/shared/pkg/resource"
	"github.com/sakuraapp/shared/pkg/resource/opcode"
	log "github.com/sirupsen/logrus"
)

type AuthResponseData struct {
	SessionId string `json:"sessionId" msgpack:"sessionId"`
}

func (h *Handlers) handleAuthFail(err error, client *client.Client) {
	log.WithError(err).Error("Authentication failed")
	client.Disconnect()
}

func (h *Handlers) HandleAuth(packet *resource.Packet, c *client.Client) gateway.Error {
	data := packet.DataMap()
	token, ok := data["token"].(string)

	if !ok || len(token) == 0 {
		c.Disconnect() // invalid token
		return nil
	}

	claims, err := h.app.GetJWT().Parse(token)

	if err != nil {
		return gateway.NewAuthError(err)
	}

	ctx := c.Context()

	fUserId := claims["id"].(float64)
	userId := model.UserId(fUserId)

	user, err := h.app.GetRepos().User.GetWithDiscriminator(ctx, userId)

	if err != nil {
		if err == pg.ErrNoRows {
			c.Disconnect()
			return nil
		} else {
			return gateway.NewAuthError(err)
		}
	}

	nodeId := h.app.NodeId()
	rdb := h.app.GetRedis()

	pipe := rdb.Pipeline()
	iSessionId := data["sessionId"]

	var key string
	s := c.Session

	if iSessionId != nil {
		sessionId := iSessionId.(string)
		var sess client.Session

		key = fmt.Sprintf(constant.SessionFmt, sessionId)
		err = rdb.HGetAll(ctx, key).Scan(&sess)

		if err == nil {
			// if userId is 0 inside the session, then it's already expired
			if sess.UserId != 0 {
				s = &sess

				if user.Id != s.UserId {
					err = fmt.Errorf("session hijack attempted: session owner %v - target user %v", s.UserId, user.Id)

					return gateway.NewAuthError(err)
				}

				s.Id = sessionId

				clientMgr := h.app.GetClientMgr()
				oldClient := clientMgr.Get(sessionId)

				if oldClient != nil {
					oldClient.Disconnect()
				}

				clientMgr.UpdateSession(c, s)

				pipe.Persist(ctx, key)

				if s.NodeId != nodeId {
					pipe.HSet(ctx, key, "node_id", nodeId)
				}
			}
		} else {
			log.
				WithField("session_id", sessionId).
				WithError(err).
				Error("Failed to reclaim session")
		}
	}

	if s.UserId == 0 {
		s.UserId = userId

		sMap := map[string]interface{}{
			"user_id": user.Id,
			"room_id": s.RoomId,
			"node_id": nodeId,
		}

		key = fmt.Sprintf(constant.SessionFmt, s.Id)

		pipe.HSet(ctx, key, sMap)
	}

	h.app.GetSessionMgr().Add(s)

	userSessionsKey := fmt.Sprintf(constant.UserSessionsFmt, user.Id)
	pipe.SAdd(ctx, userSessionsKey, s.Id)

	_, err = pipe.Exec(ctx)

	if err != nil {
		return gateway.NewAuthError(err)
	}

	userTopic := dispatcher.NewUserTarget(s.UserId).Build()
	sessionTopic := dispatcher.NewSessionTarget(s.Id).Build()

	subMgr := h.app.GetSubscriptionMgr()
	err = subMgr.AddMulti(ctx, []string{
		userTopic,
		sessionTopic,
	}, c)

	if err != nil {
		return gateway.NewAuthError(err)
	}

	log.Debugf("User: %+v", user)

	err = c.Send(opcode.Authenticate, AuthResponseData{SessionId: s.Id})

	if err != nil {
		return gateway.NewAuthError(err)
	}

	if s.RoomId != 0 {
		h.HandleJoinRoom(
			&resource.Packet{
				Opcode: opcode.JoinRoom,
				Data:   float64(s.RoomId),
			},
			c,
		)
	}

	return nil
}

func (h *Handlers) HandleDisconnect(data *resource.Packet, c *client.Client) gateway.Error {
	h.removeClient(c, false)

	log.Debugf("OnDisconnect: %v", c.Session.Id)

	s := c.Session

	ctx := h.app.Context()
	rdb := h.app.GetRedis()
	pipe := rdb.Pipeline()

	userSessionsKey := fmt.Sprintf(constant.UserSessionsFmt, s.UserId)
	sessionKey := fmt.Sprintf(constant.SessionFmt, s.Id)

	pipe.SRem(ctx, userSessionsKey, s.Id)
	pipe.Expire(ctx, sessionKey, client.SessionExpiryDuration)

	_, err := pipe.Exec(ctx)

	if err != nil {
		log.WithError(err).
			WithField("session_id", s.Id).
			Error("Failed to destroy session")
	}

	userTopic := dispatcher.NewUserTarget(s.UserId).Build()
	sessionTopic := dispatcher.NewSessionTarget(s.Id).Build()

	subMgr := h.app.GetSubscriptionMgr()
	err = subMgr.RemoveMulti(ctx, []string{
		userTopic,
		sessionTopic,
	}, c)

	if err != nil {
		log.WithError(err).
			WithField("session_id", s.Id).
			Error("Failed to cleanup disconnected session")
	}

	return nil
}
