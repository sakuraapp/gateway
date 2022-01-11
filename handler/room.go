package handler

import (
	"fmt"
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/shared/constant"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
	"github.com/sakuraapp/shared/resource/permission"
	"github.com/sakuraapp/shared/resource/role"
	log "github.com/sirupsen/logrus"
	"strconv"
)

type KickMessage struct {
	UserId model.UserId
	RoomId model.RoomId
}

func (h *Handlers) HandleJoinRoom(data *resource.Packet, c *client.Client) {
	fRoomId, ok := data.Data.(float64)

	if !ok {
		return
	}

	ctx := c.Context()

	roomId := model.RoomId(fRoomId)
	room, err := h.app.GetRepos().Room.Get(ctx, roomId)

	if err != nil {
		panic(err)
	}

	s := c.Session
	currRoomId := s.RoomId
	alreadyInRoom := currRoomId == roomId
	isRoomOwner := s.UserId == room.OwnerId

	if currRoomId != 0 && !alreadyInRoom {
		h.HandleLeaveRoom(data, c)
	}

	userId := s.UserId
	strUserId := strconv.FormatInt(int64(userId), 10)

	rdb := h.app.GetRedis()

	if room.Private && !alreadyInRoom && !isRoomOwner {
		inviteKey := fmt.Sprintf(constant.RoomInviteFmt, roomId)
		inviteExists, err := rdb.HExists(ctx, inviteKey, strUserId).Result()

		if err != nil || !inviteExists {
			reqMsg := resource.ServerMessage{
				Type: resource.NORMAL_MESSAGE,
				Target: resource.MessageTarget{
					UserIds: []model.UserId{userId},
				},
				Data: resource.Packet{
					Opcode: opcode.RoomJoinRequest,
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

	sessionId := s.Id

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

	log.Debugf("Join Room: %+v", room)

	s.RoomId = roomId

	m := h.app.GetRoomMgr()
	r := m.Get(roomId)

	if r == nil {
		r = m.Create(roomId)
	}

	err = r.Add(c)

	if err != nil {
		panic(err)
	}

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

	roomMembers, err := h.app.GetRepos().User.GetRoomMembers(userIds, roomId)

	if err != nil {
		panic(err)
	}

	members := make([]*resource.RoomMember, 0, len(roomMembers))

	for _, roomMember := range roomMembers {
		member := resource.NewRoomMember(&roomMember)
		member.Roles = append(member.Roles, role.MEMBER)

		if member.User.Id == room.OwnerId {
			member.Roles = append(member.Roles, role.HOST)
		}

		members = append(members, member)
	}

	addUserMessage := resource.ServerMessage{
		Data: resource.BuildPacket(opcode.AddUser, members[0]),
		Target: resource.MessageTarget{
			IgnoredSessionIds: map[string]bool{sessionId: true},
		},
	}

	userRoles, err := h.app.GetRepos().Role.Get(userId, roomId)

	if err != nil {
		panic(err)
	}

	roles := model.BuildRoleManager(userRoles)

	c.Session.Roles = roles

	joinRoomData := map[string]interface{}{
		"status": 200,
		"room": resource.NewRoom(room),
		"members": members,
		"roles": roles.Slice(),
		"permissions": roles.Permissions(),
	}

	err = h.app.DispatchRoom(roomId, addUserMessage)

	if err != nil {
		panic(err)
	}

	err = c.Send(opcode.JoinRoom, joinRoomData)

	if err != nil {
		panic(err)
	}

	currentItemKey := fmt.Sprintf(constant.RoomCurrentItemFmt, roomId)

	vals, err := rdb.HGetAll(ctx, currentItemKey).Result()

	if err != nil {
		fmt.Println(err)
		return
	}

	intAuthor := int64(0)

	if vals["author"] != "" {
		intAuthor, err = strconv.ParseInt(vals["author"], 10, 64)
	}

	if err != nil {
		panic(err)
	}

	currentItem := resource.MediaItem{
		Id: vals["id"],
		Author: model.UserId(intAuthor),
		MediaItemInfo: &resource.MediaItemInfo{
			Title: vals["title"],
			Icon: vals["icon"],
			Url: vals["url"],
		},
	}

	if currentItem.Id != "" {
		err = c.Send(opcode.VideoSet, currentItem)

		if err != nil {
			panic(err)
		}

		err = h.sendStateToClient(c)

		if err != nil {
			panic(err)
		}
	}
}

func (h *Handlers) removeClient(c *client.Client, updateSession bool)  {
	s := c.Session
	userId := s.UserId
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
	userSessionsKey := fmt.Sprintf(constant.RoomUserSessionsFmt, roomId, userId)
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
		err = rdb.SRem(ctx, usersKey, userId).Err()

		if err != nil {
			panic(err)
		}

		leaveMsg := resource.ServerMessage{
			Data: resource.BuildPacket(opcode.RemoveUser, userId),
		}

		err = h.app.DispatchRoom(roomId, leaveMsg)

		if err != nil {
			panic(err)
		}
	}

	s.RoomId = 0
}

func (h *Handlers) HandleKickUser(data *resource.Packet, c *client.Client)  {
	s := c.Session
	roomId := s.RoomId

	if roomId == 0 || !s.Roles.HasPermission(permission.KICK_MEMBERS) {
		log.
			WithFields(log.Fields{
				"user_id": s.UserId,
				"room_id": s.RoomId,
			}).
			Warn("Attempted to kick a user without the correct permissions")

		return
	}

	fUserId, ok := data.Data.(float64)

	if !ok {
		return
	}

	targetUserId := model.UserId(fUserId)

	if targetUserId == s.UserId {
		return
	}

	ctx := c.Context()
	rdb := h.app.GetRedis()

	usersKey := fmt.Sprintf(constant.RoomUsersFmt, roomId)

	isInRoom, err := rdb.SIsMember(ctx, usersKey, targetUserId).Result()

	if err != nil {
		panic(err)
	}

	if !isInRoom {
		return
	}

	userRoles, err := h.app.GetRepos().Role.Get(targetUserId, roomId)

	if err != nil {
		panic(err)
	}

	roles := model.BuildRoleManager(userRoles)

	myHighestRole := s.Roles.Max()
	hisHighestRole := roles.Max()

	if myHighestRole.Order() <= hisHighestRole.Order() {
		log.
			WithFields(log.Fields{
				"user_id": s.UserId,
				"target_user_id": targetUserId,
			}).
			Warn("User tried to kick another user with an equal or higher authority")
		return
	}

	userSessionsKey := fmt.Sprintf(constant.RoomUserSessionsFmt, roomId, targetUserId)
	sessions, err := rdb.SMembers(ctx, userSessionsKey).Result()

	if err != nil {
		panic(err)
	}

	kickMsg := resource.ServerMessage{
		Type: resource.SERVER_MESSAGE,
		Target: resource.MessageTarget{
			SessionIds: sessions,
		},
		Data: resource.Packet{
			Opcode: opcode.KickUser,
			Data: KickMessage{
				UserId: targetUserId,
				RoomId: roomId,
			},
		},
	}

	err = h.app.Dispatch(kickMsg)

	if err != nil {
		panic(err)
	}

	pipe := rdb.Pipeline()

	pipe.SRem(ctx, usersKey, targetUserId)
	pipe.Del(ctx, userSessionsKey)

	for _, session := range sessions {
		sessionKey := fmt.Sprintf(constant.SessionFmt, session)
		pipe.HSet(ctx, sessionKey, "room_id", 0)
	}

	_, err = pipe.Exec(ctx)

	if err != nil {
		panic(err)
	}

	leaveMsg := resource.ServerMessage{
		Data: resource.BuildPacket(opcode.RemoveUser, targetUserId),
	}

	err = h.app.DispatchRoom(roomId, leaveMsg)

	if err != nil {
		panic(err)
	}
}

func (h *Handlers) HandleLeaveRoom(data *resource.Packet, c *client.Client) {
	h.removeClient(c, true)
}

func (h *Handlers) kickUser(msg *resource.Packet) {
	data, ok := msg.Data.(KickMessage)

	if !ok {
		return
	}

	userId := data.UserId
	roomId := data.RoomId

	m := h.app.GetRoomMgr()

	clients := h.app.GetClientMgr().Clients()
	sessions := h.app.GetSessionMgr().GetByUserId(userId)

	var err error

	for sessionId, s := range sessions {
		if s.RoomId != roomId {
			continue
		}

		c := clients[sessionId]
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

		s.RoomId = 0

		err = c.Send(opcode.KickUser, nil)

		if err != nil {
			panic(err)
		}
	}
}