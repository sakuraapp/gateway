package handler

import (
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/sakuraapp/gateway/internal/client"
	"github.com/sakuraapp/gateway/internal/gateway"
	"github.com/sakuraapp/shared/pkg/constant"
	dispatcher "github.com/sakuraapp/shared/pkg/dispatcher/gateway"
	"github.com/sakuraapp/shared/pkg/model"
	"github.com/sakuraapp/shared/pkg/resource"
	"github.com/sakuraapp/shared/pkg/resource/opcode"
	"github.com/sakuraapp/shared/pkg/resource/permission"
	"github.com/sakuraapp/shared/pkg/resource/role"
	log "github.com/sirupsen/logrus"
	"strconv"
)

type RoleUpdateMessage struct {
	RoomId model.RoomId `json:"roomId" mapstructure:"roomId"`
	UserId model.UserId `json:"userId" mapstructure:"userId"`
	RoleId role.Id      `json:"roleId" mapstructure:"roleId"`
}

type KickUserMessage struct {
	RoomId model.RoomId `json:"roomId" mapstructure:"roomId"`
	UserId model.UserId `json:"userId" mapstructure:"userId"`
}

func (h *Handlers) HandleJoinRoom(data *resource.Packet, c *client.Client) gateway.Error {
	fRoomId, ok := data.Data.(float64)

	if !ok {
		return nil
	}

	ctx := c.Context()

	roomId := model.RoomId(fRoomId)
	room, err := h.app.GetRepos().Room.Get(ctx, roomId)

	if err != nil {
		return gateway.NewError(gateway.ErrorDatabase, err)
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
				return gateway.NewError(gateway.ErrorRedis, err)
			}
		} else if err == redis.Nil {
			// this runs if a request did not exist at all
			var user *model.User
			user, err = h.app.GetRepos().User.FetchWithDiscriminator(userId)

			if err != nil {
				return gateway.NewError(gateway.ErrorDatabase, err)
			}

			err = rdb.HSet(ctx, joinRequestsKey, strUserId, "0").Err()

			if err != nil {
				return gateway.NewError(gateway.ErrorRedis, err)
			}

			reqMsg := dispatcher.Message{
				Filters: dispatcher.NewFilterMap().WithPermissions(permission.MANAGE_ROOM),
				Payload: resource.Packet{
					Opcode: opcode.AddNotification,
					Data: resource.Notification{
						Id:   uuid.NewString(),
						Type: resource.NotificationJoinRequest,
						Data: builder.NewUser(user),
					},
				},
			}

			err = h.app.DispatchTo(dispatcher.NewRoomTarget(roomId), &reqMsg)

			if err != nil {
				return gateway.NewError(gateway.ErrorDispatch, err)
			}

			return nil
		} else {
			// this runs if there was an error, or an existing request

			if err != nil {
				return gateway.NewError(gateway.ErrorRedis, err)
			}

			return nil // don't send a new request if there's an existing one
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
		return gateway.NewError(gateway.ErrorRedis, err)
	}

	m := h.app.GetRoomMgr()
	r := m.Get(roomId)

	if r == nil {
		r, err = m.Create(ctx, roomId)

		if err != nil {
			return gateway.NewError(gateway.ErrorAddClient, err)
		}
	}

	r.Add(c)

	strUserIds, err := rdb.SMembers(ctx, usersKey).Result()

	if err != nil {
		return gateway.NewError(gateway.ErrorRedis, err)
	}

	// todo: make this code not awful
	userCount := len(strUserIds)
	userIds := make([]model.UserId, 0, userCount)
	userIds = append(userIds, userId) // add current user at the front so we can find their user object easily

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
		return gateway.NewError(gateway.ErrorDatabase, err)
	}

	members := make([]*resource.RoomMember, 0, len(roomMembers))

	for _, roomMember := range roomMembers {
		member := builder.NewRoomMember(&roomMember)
		members = append(members, member)
	}

	addUserMessage := dispatcher.Message{
		Payload: resource.BuildPacket(opcode.AddUser, members[0]),
		Filters: dispatcher.NewFilterMap().WithIgnoredSession(sessionId),
	}

	userRoles := members[0].Roles
	roles := role.NewManager()

	for _, roleId := range userRoles {
		roles.Add(roleId)
	}

	c.Session.Roles = roles

	joinRoomData := map[string]interface{}{
		"status":      200,
		"room":        builder.NewRoom(room),
		"members":     members,
		"permissions": roles.Permissions(),
	}

	err = h.app.DispatchTo(dispatcher.NewRoomTarget(roomId), &addUserMessage)

	if err != nil {
		return gateway.NewError(gateway.ErrorDispatch, err)
	}

	err = c.Send(opcode.JoinRoom, joinRoomData)

	if err != nil {
		return gateway.NewError(gateway.ErrorClientSend, err)
	}

	currentItemKey := fmt.Sprintf(constant.RoomCurrentItemFmt, roomId)

	vals, err := rdb.HGetAll(ctx, currentItemKey).Result()

	if err == redis.Nil || len(vals) == 0 {
		return nil
	} else if err != nil {
		return gateway.NewError(gateway.ErrorRedis, err)
	}

	intAuthor := int64(0)

	if vals["author"] != "" {
		intAuthor, err = strconv.ParseInt(vals["author"], 10, 64)

		if err != nil {
			return gateway.NewError(gateway.ErrorParse, err)
		}
	}

	intType, err := strconv.ParseInt(vals["type"], 10, 64)

	if err != nil {
		return gateway.NewError(gateway.ErrorParse, err)
	}

	currentItem := resource.MediaItem{
		Id:     vals["id"],
		Author: model.UserId(intAuthor),
		Type:   resource.MediaItemType(intType),
		MediaItemInfo: &resource.MediaItemInfo{
			Title: vals["title"],
			Icon:  vals["icon"],
			Url:   vals["url"],
		},
	}

	if currentItem.Id != "" {
		err = c.Send(opcode.VideoSet, currentItem)

		if err != nil {
			return gateway.NewError(gateway.ErrorClientSend, err)
		}

		err = h.sendStateToClient(c)

		if err != nil {
			return gateway.NewError(gateway.ErrorSendState, err)
		}
	}

	return nil
}

func (h *Handlers) HandleUpdateRole(data *resource.Packet, c *client.Client) gateway.Error {
	s := c.Session
	roomId := s.RoomId

	if roomId == 0 || !s.HasPermission(permission.MANAGE_ROLES) {
		log.
			WithFields(log.Fields{
				"user_id": s.UserId,
				"room_id": s.RoomId,
			}).
			Warn("Attempted to update a user's roles without the correct permissions")

		return nil
	}

	var opts RoleUpdateMessage

	err := mapstructure.Decode(data.Data, &opts)

	if err != nil {
		return gateway.NewError(gateway.ErrorParse, err)
	}

	if opts.UserId == s.UserId {
		return nil
	}

	r := role.GetRole(opts.RoleId)

	if r == nil {
		return nil
	}

	myHighestRole := s.Roles.Max()

	if r.Order() >= myHighestRole.Order() {
		return nil
	}

	ctx := c.Context()
	rdb := h.app.GetRedis()

	usersKey := fmt.Sprintf(constant.RoomUsersFmt, roomId)
	isInRoom, err := rdb.SIsMember(ctx, usersKey, opts.UserId).Result()

	if err != nil {
		return gateway.NewError(gateway.ErrorRedis, err)
	}

	if !isInRoom {
		return nil
	}

	roleRepo := h.app.GetRepos().Role

	if data.Opcode == opcode.RemoveRole {
		userRoles, err := roleRepo.Get(opts.UserId, roomId)

		if err != nil {
			return gateway.NewError(gateway.ErrorDatabase, err)
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

			return nil
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
		return gateway.NewError(gateway.ErrorDatabase, err)
	}

	updateServerMsg := dispatcher.Message{
		Payload: resource.BuildPacket(data.Opcode, &RoleUpdateMessage{
			RoomId: roomId,
			UserId: opts.UserId,
			RoleId: opts.RoleId,
		}),
		Filters: dispatcher.NewFilterMap().WithType(dispatcher.ServerMessage).WithRoom(roomId),
	}

	err = h.app.DispatchTo(dispatcher.NewUserTarget(opts.UserId), &updateServerMsg)

	if err != nil {
		return gateway.NewError(gateway.ErrorDispatch, err)
	}

	updateMsg := dispatcher.Message{
		Payload: *data,
		Filters: dispatcher.NewFilterMap().WithIgnoredSession(s.Id),
	}

	err = h.app.DispatchTo(dispatcher.NewRoomTarget(roomId), &updateMsg)

	if err != nil {
		return gateway.NewError(gateway.ErrorDispatch, err)
	}

	return nil
}

func (h *Handlers) UpdateRole(msg *dispatcher.Message) {
	var opts RoleUpdateMessage

	err := mapstructure.Decode(msg.Payload.Data, &opts)

	if err != nil {
		log.WithError(err).Error("Failed to parse update role message")
		return
	}

	userId := opts.UserId
	roleId := opts.RoleId
	roomId := opts.RoomId

	ignoredSessionId := msg.Filters[dispatcher.MessageFilterIgnoredSession]

	clients := h.app.GetClientMgr().Clients()
	sessions := h.app.GetSessionMgr().GetByUserId(userId)

	for _, s := range sessions {
		if s.RoomId != roomId || ignoredSessionId == s.Id {
			continue
		}

		c := clients[s.Id]

		s.Roles.Add(roleId)
		err = c.Send(opcode.UpdatePermissions, s.Roles.Permissions())

		if err != nil {
			log.WithField("session_id", s.Id).
				WithError(err).
				Error("Failed to send update permissions message")
		}
	}
}

func (h *Handlers) removeClient(c *client.Client, updateSession bool) error {
	s := c.Session

	userId := s.UserId
	roomId := s.RoomId

	if roomId == 0 {
		return nil
	}

	var err error

	m := h.app.GetRoomMgr()
	r := m.Get(roomId)

	if r != nil {
		r.Remove(c)

		if r.NumClients() == 0 {
			err = m.Delete(h.app.Context(), roomId)

			if err != nil {
				return err
			}
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
		return err
	}

	sessionCount, err := rdb.SCard(ctx, userSessionsKey).Result()

	if err != nil {
		return err
	}

	if sessionCount == 0 {
		err = rdb.SRem(ctx, usersKey, userId).Err()

		if err != nil {
			return err
		}

		leaveMsg := dispatcher.Message{
			Payload: resource.BuildPacket(opcode.RemoveUser, userId),
		}

		err = h.app.DispatchTo(dispatcher.NewRoomTarget(roomId), &leaveMsg)

		if err != nil {
			return err
		}
	}

	s.RoomId = 0

	return nil
}

func (h *Handlers) HandleKickUser(data *resource.Packet, c *client.Client) gateway.Error {
	s := c.Session
	roomId := s.RoomId

	if roomId == 0 || !s.HasPermission(permission.KICK_MEMBERS) {
		log.
			WithFields(log.Fields{
				"user_id": s.UserId,
				"room_id": s.RoomId,
			}).
			Warn("Attempted to kick a user without the correct permissions")

		return nil
	}

	fUserId, ok := data.Data.(float64)

	if !ok {
		return nil
	}

	targetUserId := model.UserId(fUserId)

	if targetUserId == s.UserId {
		return nil
	}

	ctx := c.Context()
	rdb := h.app.GetRedis()

	usersKey := fmt.Sprintf(constant.RoomUsersFmt, roomId)

	isInRoom, err := rdb.SIsMember(ctx, usersKey, targetUserId).Result()

	if err != nil {
		return gateway.NewError(gateway.ErrorRedis, err)
	}

	if !isInRoom {
		return nil
	}

	userRoles, err := h.app.GetRepos().Role.Get(targetUserId, roomId)

	if err != nil {
		return gateway.NewError(gateway.ErrorDatabase, err)
	}

	roles := model.BuildRoleManager(userRoles)

	myHighestRole := s.Roles.Max()
	hisHighestRole := roles.Max()

	if myHighestRole.Order() <= hisHighestRole.Order() {
		log.
			WithFields(log.Fields{
				"user_id":        s.UserId,
				"target_user_id": targetUserId,
			}).
			Warn("User tried to kick another user with an equal or higher authority")

		return nil
	}

	userSessionsKey := fmt.Sprintf(constant.RoomUserSessionsFmt, roomId, targetUserId)
	sessions, err := rdb.SMembers(ctx, userSessionsKey).Result()

	if err != nil {
		return gateway.NewError(gateway.ErrorRedis, err)
	}

	kickMsg := dispatcher.Message{
		Filters: dispatcher.NewFilterMap().WithType(dispatcher.ServerMessage).WithRoom(roomId),
		Payload: resource.Packet{
			Opcode: opcode.KickUser,
			Data: &KickUserMessage{
				UserId: targetUserId,
				RoomId: roomId,
			},
		},
	}

	err = h.app.DispatchTo(dispatcher.NewUserTarget(targetUserId), &kickMsg)

	if err != nil {
		return gateway.NewError(gateway.ErrorDispatch, err)
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
		return gateway.NewError(gateway.ErrorRedis, err)
	}

	leaveMsg := dispatcher.Message{
		Payload: resource.BuildPacket(opcode.RemoveUser, targetUserId),
	}

	err = h.app.DispatchTo(dispatcher.NewRoomTarget(roomId), &leaveMsg)

	if err != nil {
		return gateway.NewError(gateway.ErrorDispatch, err)
	}

	return nil
}

func (h *Handlers) HandleLeaveRoom(data *resource.Packet, c *client.Client) gateway.Error {
	err := h.removeClient(c, true)

	if err != nil {
		return gateway.NewError(gateway.ErrorRemoveClient, err)
	}

	return nil
}

func (h *Handlers) KickUser(msg *dispatcher.Message) {
	var opts KickUserMessage

	err := mapstructure.Decode(msg.Payload.Data, &opts)

	if err != nil {
		log.WithError(err).Error("Failed to parse kick message")
		return
	}

	userId := opts.UserId
	roomId := opts.RoomId

	m := h.app.GetRoomMgr()
	r := m.Get(roomId)

	if r == nil {
		return
	}

	clients := h.app.GetClientMgr().Clients()
	sessions := h.app.GetSessionMgr().GetByUserId(userId)

	var c *client.Client

	logger := log.WithField("room_id", roomId)

	for sessionId, s := range sessions {
		if s.RoomId != roomId {
			continue
		}

		c = clients[sessionId]
		r.Remove(c)

		if r.NumClients() == 0 {
			err = m.Delete(h.app.Context(), roomId)

			if err != nil {
				logger.WithField("session_id", sessionId).
					WithError(err).
					Error("Failed to delete empty room")
			}
		}

		s.RoomId = 0

		err = c.Send(opcode.KickUser, nil)

		if err != nil {
			logger.WithField("session_id", sessionId).
				WithError(err).
				Error("Failed to send kick message to a user")
		}
	}
}

func (h *Handlers) HandleAcceptRoomJoinRequest(data *resource.Packet, c *client.Client) gateway.Error {
	s := c.Session
	roomId := s.RoomId

	if roomId == 0 || !c.Session.HasPermission(permission.MANAGE_ROOM) {
		log.
			WithFields(log.Fields{
				"user_id": s.UserId,
				"room_id": roomId,
			}).
			Warn("Attempted to accept a user's join request without the correct permissions")

		return nil
	}

	fUserId, ok := data.Data.(float64)

	if !ok {
		return nil
	}

	targetUserId := model.UserId(fUserId)
	strUserId := strconv.FormatInt(int64(fUserId), 10)

	ctx := c.Context()
	rdb := h.app.GetRedis()

	joinRequestsKey := fmt.Sprintf(constant.RoomJoinRequestsFmt, roomId)

	err := rdb.HSet(ctx, joinRequestsKey, strUserId, "1").Err()

	if err != nil {
		return gateway.NewError(gateway.ErrorRedis, err)
	}

	msg := dispatcher.Message{
		Payload: resource.BuildPacket(opcode.RoomJoinRequest, roomId),
	}

	err = h.app.DispatchTo(dispatcher.NewUserTarget(targetUserId), &msg)

	if err != nil {
		return gateway.NewError(gateway.ErrorDispatch, err)
	}

	return nil
}
