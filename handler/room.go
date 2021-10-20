package handler

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/gateway/internal"
	"github.com/sakuraapp/shared/constant"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
	"github.com/sakuraapp/shared/resource/permission"
	"net/url"
	"strconv"
	"time"
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

	s := c.Session
	currRoomId := s.RoomId
	alreadyInRoom := currRoomId == roomId

	if currRoomId != 0 && !alreadyInRoom {
		h.HandleLeaveRoom(data, c)
	}

	userId := s.UserId
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

	fmt.Printf("Join Room: %+v\n", room)

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

	users, err := h.app.GetRepos().User.GetUsersWithDiscriminators(userIds)

	if err != nil {
		panic(err)
	}

	addUserMessage := resource.ServerMessage{
		Data: resource.BuildPacket(opcode.AddUser, resource.NewUser(&users[0])),
		Target: resource.MessageTarget{
			IgnoredSessionIds: map[string]bool{sessionId: true},
		},
	}

	s.Permissions = permission.QUEUE_ADD

	if userId == room.OwnerId {
		s.AddPermission(permission.QUEUE_EDIT)
		s.AddPermission(permission.VIDEO_REMOTE)
	}

	joinRoomData := map[string]interface{}{
		"status": 200,
		"room": resource.NewRoom(room),
		"users": resource.NewUserList(users),
		"permissions": s.Permissions,
	}

	err = h.app.DispatchRoom(roomId, addUserMessage)

	if err != nil {
		panic(err)
	}

	err = c.Send(opcode.JoinRoom, joinRoomData)

	if err != nil {
		panic(err)
	}

	err = h.sendStateToClient(c)

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

func (h *Handlers) HandleQueueAdd(data *resource.Packet, c *client.Client) {
	roomId := c.Session.RoomId

	if roomId == 0 || !c.Session.HasPermission(permission.QUEUE_ADD) {
		return
	}

	rawUrl := data.Data.(string)
	u, err := url.Parse(rawUrl)

	if err != nil {
		return
	}

	switch internal.GetDomain(u) {
	case "youtube.com":
		videoId := u.Query().Get("v")
		rawUrl = fmt.Sprintf("https://www.youtube.com/embed/%v", videoId)
	}

	itemInfo, err := h.app.GetCrawler().Get(rawUrl)

	if err != nil {
		panic(err)
	}

	item := resource.MediaItem{
		Id: uuid.NewString(),
		MediaItemInfo: itemInfo,
	}

	queueKey := fmt.Sprintf(constant.RoomQueueFmt, roomId)
	currentItemKey := fmt.Sprintf(constant.RoomCurrentItemFmt, roomId)

	ctx := c.Context()
	rdb := h.app.GetRedis()

	pipe := rdb.Pipeline()

	lenCmd := pipe.LLen(ctx, queueKey)
	currentCmd := pipe.HExists(ctx, currentItemKey, "url")

	_, err = pipe.Exec(ctx)

	if err != nil {
		panic(err)
	}

	if lenCmd.Val() > 0 || currentCmd.Val() {
		// something else is already playing
		err = rdb.LPush(ctx, queueKey, item).Err()

		if err != nil {
			panic(err)
		}
	} else {
		h.setCurrentItem(h.app.Context(), roomId, item)
	}
}

func (h *Handlers) nextItem(ctx context.Context, roomId model.RoomId) {
	rdb := h.app.GetRedis()
	queueKey := fmt.Sprintf(constant.RoomQueueFmt, roomId)

	var item resource.MediaItem

	err := rdb.LPop(ctx, queueKey).Scan(&item)

	if err != nil {
		fmt.Printf("Error playing next queue item: %v\n", err.Error())
		return
	}

	queueRemoveMsg := resource.ServerMessage{
		Data: resource.BuildPacket(opcode.QueueRemove, item.Id),
	}

	err = h.app.DispatchRoom(roomId, queueRemoveMsg)

	if err != nil {
		panic(err)
	}

	h.setCurrentItem(ctx, roomId, item)
}

func (h *Handlers) setCurrentItem(ctx context.Context, roomId model.RoomId, item resource.MediaItem) {
	rdb := h.app.GetRedis()

	currentItemKey := fmt.Sprintf(constant.RoomCurrentItemFmt, roomId)
	stateKey := fmt.Sprintf(constant.RoomStateFmt, roomId)

	state := resource.PlayerState{
		IsPlaying: false,
		CurrentTime: 0 * time.Millisecond,
	}

	err := h.dispatchState(roomId, &state)

	if err != nil {
		panic(err)
	}

	pipe := rdb.Pipeline()

	pipe.HSet(ctx, currentItemKey, item)
	pipe.HSet(ctx, stateKey, state)

	_, err = pipe.Exec(ctx)

	if err != nil {
		panic(err)
	}
}

func (h *Handlers) getState(ctx context.Context, roomId model.RoomId) (*resource.PlayerState, error) {
	rdb := h.app.GetRedis()
	stateKey := fmt.Sprintf(constant.RoomStateFmt, roomId)

	var state resource.PlayerState

	err := rdb.HGetAll(ctx, stateKey).Scan(&state)

	if err != nil {
		return nil, err
	}

	if state.IsPlaying {
		timeDiff := time.Now().Sub(state.PlaybackStart)
		state.CurrentTime += timeDiff
	}

	return &state, nil
}

func (h *Handlers) dispatchState(roomId model.RoomId, state *resource.PlayerState) error {
	stateMsg := resource.ServerMessage{
		Data: state.BuildPacket(),
	}

	return h.app.DispatchRoom(roomId, stateMsg)
}

func (h *Handlers) sendState(ctx context.Context, roomId model.RoomId) error {
	state, err := h.getState(ctx, roomId)

	if err != nil {
		return err
	}

	return h.dispatchState(roomId, state)
}

func (h *Handlers) sendStateToClient(c *client.Client) error {
	state, err := h.getState(c.Context(), c.Session.RoomId)

	if err != nil {
		return err
	}

	stateMsg := resource.ServerMessage{
		Type: resource.NORMAL_MESSAGE,
		Data: state.BuildPacket(),
		Target: resource.MessageTarget{
			UserIds: map[model.UserId]bool{c.Session.UserId: true},
		},
	}

	err = h.app.Dispatch(stateMsg)

	if err != nil {
		return err
	}

	return nil
}