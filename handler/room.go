package handler

import (
	"fmt"
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/shared/constant"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
	"github.com/sakuraapp/shared/resource/permission"
	"strconv"
)

func (h *Handlers) HandleJoinRoom(data *resource.Packet, c *client.Client) {
	strRoomId := data.Data.(string)
	intRoomId, err := strconv.Atoi(strRoomId)

	if err != nil {
		panic(err)
	}

	roomId := model.RoomId(intRoomId)
	room, err := h.app.GetRepos().Room.Get(roomId)

	if err != nil {
		panic(err)
	}

	currRoomId := c.Session.RoomId
	alreadyInRoom := currRoomId == roomId

	if currRoomId != 0 && !alreadyInRoom {
		h.HandleLeaveRoom(data, c)
	}

	userId := c.Session.UserId
	strUserId := string(userId)

	ctx := c.Context()
	rdb := h.app.GetRedis()

	if room.Private && !alreadyInRoom {
		inviteKey := fmt.Sprintf(constant.RoomInviteFmt, roomId)
		inviteExists, err := rdb.HExists(ctx, inviteKey, strUserId).Result()

		if err != nil || !inviteExists {
			reqMsg := resource.ServerMessage{
				Type: resource.NORMAL_MESSAGE,
				Target: resource.MessageTarget{
					UserIds: map[model.UserId]bool{
						userId: true,
					},
				},
				Data: resource.Packet{
					Opcode: opcode.ROOM_JOIN_REQUEST,
					Data: userId,
				},
			}

			err = h.app.Dispatch(reqMsg)

			if err != nil {
				panic(err)
			}

			return
		} else {
			err = rdb.HDel(ctx, inviteKey, strUserId).Err()

			if err != nil {
				panic(err)
			}
		}
	}

	sessionId := c.Session.Id

	usersKey := fmt.Sprintf(constant.RoomUsersFmt, roomId)
	userSessionsKey := fmt.Sprintf(constant.RoomUserSessionsFmt, roomId, userId)
	sessionKey := fmt.Sprintf(constant.SessionFmt, sessionId)

	pipe := rdb.Pipeline()

	pipe.SAdd(ctx, usersKey, userId)
	pipe.SAdd(ctx, userSessionsKey, sessionId)
	pipe.HSet(ctx, sessionKey, "room_id", roomId)

	_, err = pipe.Exec(ctx)

	if err != nil {
		panic(err)
	}

	fmt.Printf("Join Room: %+v\n", room)

	c.Session.RoomId = roomId

	strUserIds, err := rdb.SMembers(ctx, usersKey).Result()

	if err != nil {
		panic(err)
	}

	// todo: make this code not awful
	userCount := len(strUserIds)
	userIds := make([]model.UserId, 0, userCount)
	userIds = append(userIds, userId)// add current user at the front so we can find their user object easily

	var intUID int
	var uid model.UserId

	for _, strUID := range strUserIds {
		if strUID == strUserId {
			continue // don't re-add current user
		}

		intUID, err = strconv.Atoi(strUID)

		if err == nil {
			uid = model.UserId(intUID)

			userIds = append(userIds, uid)
		}
	}

	users, err := h.app.GetRepos().User.GetUsersWithDiscriminators(userIds)

	addUserMessage := resource.ServerMessage{
		Data: resource.BuildPacket(opcode.ADD_USER, resource.NewUser(&users[0])),
		Target: resource.MessageTarget{
			IgnoredSessionIds: map[string]bool{sessionId: true},
		},
	}

	permissions := []permission.Permission{permission.QUEUE_ADD}

	if userId == room.OwnerId {
		permissions = append(permissions, permission.QUEUE_EDIT, permission.VIDEO_REMOTE)
	}

	joinRoomData := map[string]interface{}{
		"status": 200,
		"room": resource.NewRoom(room),
		"users": resource.NewUserList(users),
		"permissions": permissions,
	}

	err = h.app.DispatchRoom(roomId, addUserMessage)

	if err != nil {
		panic(err)
	}

	err = c.Send(opcode.JOIN_ROOM, joinRoomData)

	if err != nil {
		panic(err)
	}
}

func (h *Handlers) removeClient(c *client.Client, updateSession bool)  {
	s := c.Session
	roomId := s.RoomId

	if roomId == 0 {
		return
	}

	var err error

	m := h.app.GetRoomMgr()
	r := m.Get(roomId)

	if r != nil {
		err = r.Remove(c)

		if err != nil {
			panic(err)
		}

		if r.NumClients() == 0 {
			m.Delete(roomId)
		}
	}

	usersKey := fmt.Sprintf(constant.RoomUsersFmt, roomId)
	userSessionsKey := fmt.Sprintf(constant.RoomUserSessionsFmt, roomId, s.UserId)
	sessionKey := fmt.Sprintf(constant.SessionFmt, s.Id)

	ctx := h.app.Context()
	rdb := h.app.GetRedis()
	pipe := rdb.Pipeline()

	pipe.SRem(ctx, userSessionsKey, s.Id)

	if updateSession {
		pipe.HSet(ctx, sessionKey, "room_id", 0)
	}

	_, err = pipe.Exec(ctx)

	if err != nil {
		panic(err)
	}

	sessionCount, err := rdb.SCard(ctx, userSessionsKey).Result()

	if err != nil {
		panic(err)
	}

	if sessionCount == 0 {
		err = rdb.SRem(ctx, usersKey, s.UserId).Err()

		if err != nil {
			panic(err)
		}
	}

	s.RoomId = 0
}

func (h *Handlers) HandleLeaveRoom(data *resource.Packet, c *client.Client) {
	h.removeClient(c, true)
}