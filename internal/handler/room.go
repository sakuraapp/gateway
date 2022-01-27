package handler

import (
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/sakuraapp/gateway/internal/client"
	"github.com/sakuraapp/shared/constant"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
	"github.com/sakuraapp/shared/resource/permission"
	"github.com/sakuraapp/shared/resource/role"
	log "github.com/sirupsen/logrus"
	"strconv"
)

type RoleUpdateMessage struct {
	UserId model.UserId `json:"userId" mapstructure:"userId"`
	RoleId role.Id `json:"roleId" mapstructure:"roleId"`
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

	builder := h.app.GetBuilder()
	rdb := h.app.GetRedis()

	if room.Private && !alreadyInRoom && !isRoomOwner {
		joinRequestsKey := fmt.Sprintf(constant.RoomJoinRequestsFmt, roomId)

		var joinRequest int
		err = rdb.HGet(ctx, joinRequestsKey, strUserId).Scan(&joinRequest)

		if joinRequest == 1 {
			err = rdb.HDel(ctx, joinRequestsKey, strUserId).Err()

			if err != nil {
				panic(err)
			}
		} else if err == redis.Nil {
			// this runs if a request did not exist at all
			var user *model.User
			user, err = h.app.GetRepos().User.FetchWithDiscriminator(userId)

			if err != nil {
				panic(err)
			}

			err = rdb.HSet(ctx, joinRequestsKey, strUserId, "0").Err()

			if err != nil {
				panic(err)
			}

			reqMsg := resource.ServerMessage{
				Target: resource.MessageTarget{
					Permissions: permission.MANAGE_ROOM,
				},
				Data: resource.Packet{
					Opcode: opcode.AddNotification,
					Data: resource.Notification{
						Id: uuid.NewString(),
						Type: resource.NotificationJoinRequest,
						Data: builder.NewUser(user),
					},
				},
			}

			err = h.app.DispatchRoom(roomId, reqMsg)

			if err != nil {
				panic(err)
			}

			return
		} else {
			// this runs if there was an error, or an existing request

			if err != nil {
				panic(err)
			}

			return // don't send a new request if there's an existing one
		}
	}

	log.Debugf("Join Room: %+v", room)

	s.RoomId = roomId // have to do this before setting any of the redis data because if the client disconnects in the middle, those requests will be canceled, so we need to reset them by handling the disconnection
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
		member := builder.NewRoomMember(&roomMember)
		members = append(members, member)
	}

	addUserMessage := resource.ServerMessage{
		Data: resource.BuildPacket(opcode.AddUser, members[0]),
		Target: resource.MessageTarget{
			IgnoredSessionIds: map[string]bool{sessionId: true},
		},
	}

	userRoles := members[0].Roles
	roles := role.NewManager()

	for _, roleId := range userRoles {
		roles.Add(roleId)
	}

	c.Session.Roles = roles

	joinRoomData := map[string]interface{}{
		"status": 200,
		"room": builder.NewRoom(room),
		"members": members,
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

func (h *Handlers) HandleUpdateRole(data *resource.Packet, c *client.Client) {
	s := c.Session
	roomId := s.RoomId

	if roomId == 0 || !s.HasPermission(permission.MANAGE_ROLES) {
		log.
			WithFields(log.Fields{
				"user_id": s.UserId,
				"room_id": s.RoomId,
			}).
			Warn("Attempted to update a user's roles without the correct permissions")

		return
	}

	var opts RoleUpdateMessage

	err := mapstructure.Decode(data.Data, &opts)

	if err != nil {
		return
	}

	if opts.UserId == s.UserId {
		return
	}

	r := role.GetRole(opts.RoleId)

	if r == nil {
		return
	}

	myHighestRole := s.Roles.Max()

	if r.Order() >= myHighestRole.Order() {
		return
	}

	ctx := c.Context()
	rdb := h.app.GetRedis()

	usersKey := fmt.Sprintf(constant.RoomUsersFmt, roomId)
	isInRoom, err := rdb.SIsMember(ctx, usersKey, opts.UserId).Result()

	if err != nil {
		panic(err)
	}

	if !isInRoom {
		return
	}

	roleRepo := h.app.GetRepos().Role

	if data.Opcode == opcode.RemoveRole {
		userRoles, err := roleRepo.Get(opts.UserId, roomId)

		if err != nil {
			panic(err)
		}

		roles := model.BuildRoleManager(userRoles)
		hisHighestRole := roles.Max()

		if myHighestRole.Order() <= hisHighestRole.Order() {
			log.
				WithFields(log.Fields{
					"user_id":        s.UserId,
					"target_user_id": opts.UserId,
				}).
				Warn("User tried to remove a role from another user with an equal or higher authority")
			return
		}
	}

	userRole := model.UserRole{
		UserId: opts.UserId,
		RoomId: roomId,
		RoleId: r.Id(),
	}

	if data.Opcode == opcode.AddRole {
		err = roleRepo.Add(&userRole)
	} else {
		err = roleRepo.Remove(&userRole)
	}

	if err != nil {
		panic(err)
	}

	updateServerMsg := resource.ServerMessage{
		Type: resource.SERVER_MESSAGE,
		Target: resource.MessageTarget{
			UserIds: []model.UserId{opts.UserId},
			RoomId: roomId,
		},
		Data: *data,
	}

	err = h.app.Dispatch(updateServerMsg)

	if err != nil {
		panic(err)
	}

	updateMsg := resource.ServerMessage{
		Target: resource.MessageTarget{
			IgnoredSessionIds: map[string]bool{
				s.Id: true,
			},
		},
		Data: *data,
	}

	err = h.app.DispatchRoom(roomId, updateMsg)
}

func (h *Handlers) UpdateRole(msg *resource.ServerMessage) {
	var opts RoleUpdateMessage

	err := mapstructure.Decode(msg.Data.Data, &opts)

	if err != nil {
		return
	}

	userId := opts.UserId
	roleId := opts.RoleId
	roomId := msg.Target.RoomId

	ignoredSessionIds := msg.Target.IgnoredSessionIds

	clients := h.app.GetClientMgr().Clients()
	sessions := h.app.GetSessionMgr().GetByUserId(userId)

	for _, s := range sessions {
		if s.RoomId != roomId || ignoredSessionIds[s.Id] {
			continue
		}

		c := clients[s.Id]

		s.Roles.Add(roleId)
		err = c.Send(opcode.UpdatePermissions, s.Roles.Permissions())

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

	if roomId == 0 || !s.HasPermission(permission.KICK_MEMBERS) {
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
			UserIds: []model.UserId{targetUserId},
			RoomId: roomId,
		},
		Data: resource.Packet{
			Opcode: opcode.KickUser,
			Data: *data,
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

func (h *Handlers) KickUser(msg *resource.ServerMessage) {
	fUserId := msg.Data.Data.(float64)
	userId := model.UserId(fUserId)
	roomId := msg.Target.RoomId

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

func (h *Handlers) HandleAcceptRoomJoinRequest(data *resource.Packet, c *client.Client) {
	s := c.Session
	roomId := s.RoomId

	if roomId == 0 || !c.Session.HasPermission(permission.MANAGE_ROOM) {
		log.
			WithFields(log.Fields{
				"user_id": s.UserId,
				"room_id": roomId,
			}).
			Warn("Attempted to accept a user's join request without the correct permissions")

		return
	}

	fUserId, ok := data.Data.(float64)

	if !ok {
		return
	}

	targetUserId := model.UserId(fUserId)
	strUserId := strconv.FormatInt(int64(fUserId), 10)

	ctx := c.Context()
	rdb := h.app.GetRedis()

	joinRequestsKey := fmt.Sprintf(constant.RoomJoinRequestsFmt, roomId)

	err := rdb.HSet(ctx, joinRequestsKey, strUserId, "1").Err()

	if err != nil {
		panic(err)
	}

	msg := resource.ServerMessage{
		Type: resource.NORMAL_MESSAGE,
		Target: resource.MessageTarget{
			UserIds: []model.UserId{targetUserId},
		},
		Data: resource.BuildPacket(opcode.RoomJoinRequest, roomId),
	}

	err = h.app.Dispatch(msg)

	if err != nil {
		panic(err)
	}
}