package handler

import (
	"fmt"
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/shared/constant"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
	"strconv"
)

type AuthResponseData struct {
	SessionId string `json:"sessionId" msgpack:"sessionId"`
}

func (h *Handlers) handleAuthFail(err error, client *client.Client) {
	fmt.Printf("Auth Failed: %v\n", err)
	client.Disconnect()
}

func (h *Handlers) HandleAuth(packet *resource.Packet, c *client.Client) {
	data := packet.DataMap()
	token := data["token"].(string)

	claims, err := h.app.GetJWT().Parse(token)

	if err != nil {
		h.handleAuthFail(err, c)
		return
	}

	ctx := c.Context()

	fUserId := claims["id"].(float64)
	userId := model.UserId(fUserId)

	user, err := h.app.GetRepos().User.GetWithDiscriminator(ctx, userId)

	if err != nil {
		h.handleAuthFail(err, c)
		return
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
			s = &sess

			if user.Id != s.UserId {
				h.handleAuthFail(
					fmt.Errorf("session hijack attempted: session owner %v - target user %v", s.UserId, user.Id),
					c,
				)
				return
			}

			s.Id = sessionId
			h.app.GetClientMgr().UpdateSession(c, s)

			pipe.Persist(ctx, key)

			if s.NodeId != nodeId {
				pipe.HSet(ctx, key, "node_id", nodeId)
			}
		} else {
			fmt.Printf("%v\n", err)
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
		panic(err)
	}

	fmt.Printf("User: %+v\n", user)

	err = c.Send(opcode.Authenticate, AuthResponseData{SessionId: s.Id})

	if err != nil {
		h.handleAuthFail(err, c)
	}

	if s.RoomId != 0 {
		h.HandleJoinRoom(
			&resource.Packet{
				Opcode: opcode.JoinRoom,
				Data: strconv.Itoa(int(s.RoomId)),
			},
			c,
		)
	}
}

func (h *Handlers) HandleDisconnect(data *resource.Packet, c *client.Client) {
	h.removeClient(c, false)

	session := c.Session

	ctx := c.Context()
	rdb := h.app.GetRedis()
	pipe := rdb.Pipeline()

	userSessionsKey := fmt.Sprintf(constant.UserSessionsFmt, session.UserId)
	sessionKey := fmt.Sprintf(constant.SessionFmt, session.Id)

	pipe.SRem(ctx, userSessionsKey, session.Id)
	pipe.Expire(ctx, sessionKey, client.SessionExpiryDuration)

	_, err := pipe.Exec(ctx)

	if err != nil {
		panic(err)
	}
}