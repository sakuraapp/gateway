package handler

import (
	"fmt"
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
)

const sessionFmt = "session.%v"

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

	rdb := h.app.GetRedis()
	iSessionId := data["sessionId"]

	var s *client.Session
	var key string

	if iSessionId != nil {
		sessionId := iSessionId.(string)
		key = fmt.Sprintf(sessionFmt, sessionId)
		err = rdb.HGetAll(ctx, key).Scan(s)

		if err == nil {
			if user.Id != s.UserId {
				h.handleAuthFail(
					fmt.Errorf("session hijack attempted: session owner %v - target user %v", s.UserId, user.Id),
					c,
				)
				return
			}

			s.Id = sessionId
			c.Session = s

			rdb.Persist(ctx, key)

			if s.RoomId.Valid {
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
		}
 	}

 	if s == nil {
		s = c.CreateSession(user.Id)
		key = fmt.Sprintf(sessionFmt, s.Id)

		rdb.HSet(ctx, key, s)
 	}

	fmt.Printf("User: %+v\n", user)

	err = c.Send(opcode.AUTHENTICATE, AuthResponseData{SessionId: s.Id})

	if err != nil {
		h.handleAuthFail(err, c)
	}
}