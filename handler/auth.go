package handler

import (
	"fmt"
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/gateway/pkg"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
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

	var s *client.Session
	var key string

	if iSessionId != nil {
		sessionId := iSessionId.(string)
		var sess client.Session

		key = fmt.Sprintf(client.SessionFmt, sessionId)
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
			c.Session = s

			pipe.Persist(ctx, key)

			if s.NodeId != nodeId {
				pipe.HSet(ctx, key, "node_id", nodeId)
			}

			if s.RoomId != 0 {
				h.HandleJoinRoom(
					&resource.Packet{
						Opcode: opcode.JOIN_ROOM,
						Data: s.RoomId,
					},
					c,
				)
			}
		} else {
			s = nil
			fmt.Printf("%v\n", err)
		}
 	}

 	if s == nil {
 		s = client.NewSession(user.Id, nodeId)
		c.Session = s

		sMap := map[string]interface{}{
			"user_id": user.Id,
			"room_id": s.RoomId,
			"node_id": nodeId,
		}

		key = fmt.Sprintf(client.SessionFmt, s.Id)

		pipe.HSet(ctx, key, sMap)
 	}

	userSessionsKey := fmt.Sprintf(pkg.UserSessionsFmt, user.Id)
	pipe.SAdd(ctx, userSessionsKey, s.Id)

	_, err = pipe.Exec(ctx)

	if err != nil {
		panic(err)
	}

	fmt.Printf("User: %+v\n", user)

	err = c.Send(opcode.AUTHENTICATE, AuthResponseData{SessionId: s.Id})

	if err != nil {
		h.handleAuthFail(err, c)
	}

	h.app.Dispatch(resource.ServerMessage{
		Type: resource.NORMAL_MESSAGE,
		Target: resource.MessageTarget{
			UserIds: map[model.UserId]bool{1: true, 2: true},
		},
		Data: resource.Packet{},
		Origin: "abcd",
	})
}