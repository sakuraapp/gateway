package handler

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/sakuraapp/gateway/internal/client"
	"github.com/sakuraapp/shared/constant"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
	"github.com/sakuraapp/shared/resource/permission"
	log "github.com/sirupsen/logrus"
	"strconv"
	"time"
)

func buildState(state *resource.PlayerState) resource.Packet {
	data := map[string]interface{}{
		"playing": state.IsPlaying,
		"currentTime": state.CurrentTime,
		"playbackStart": state.PlaybackStart,
	}

	return resource.BuildPacket(opcode.PlayerState, data)
}

func (h *Handlers) HandleSetPlayerState(data *resource.Packet, c *client.Client) {
	if c.Session.HasPermission(permission.VIDEO_REMOTE) {
		var t time.Time

		if !data.Time.IsZero() {
			t = time.Now()
		} else {
			t = time.Unix(data.Time.Int64, 0)
		}

		ctx := h.app.Context()
		rdb := h.app.GetRedis()

		m := data.DataMap()
		state := resource.PlayerState{
			IsPlaying: m["playing"].(bool),
			CurrentTime: m["currentTime"].(float64),
			PlaybackStart: t,
		}

		msg := resource.ServerMessage{
			Data: resource.BuildPacket(opcode.PlayerState, state),
			Target: resource.MessageTarget{
				IgnoredSessionIds: map[string]bool{c.Session.Id: true},
			},
		}

		roomId := c.Session.RoomId
		stateKey := fmt.Sprintf(constant.RoomStateFmt, roomId)

		err := h.app.DispatchRoom(roomId, msg)

		if err != nil {
			panic(err)
		}

		err = rdb.HSet(ctx,
			stateKey,
			"playing",
			state.IsPlaying,
			"currentTime",
			state.CurrentTime,
			"playbackStart",
			state.PlaybackStart,
		).Err()

		if err != nil {
			panic(err)
		}
	}
}

func (h *Handlers) HandleSeek(data *resource.Packet, c *client.Client) {
	if c.Session.HasPermission(permission.VIDEO_REMOTE) {
		ctx := h.app.Context()
		rdb := h.app.GetRedis()

		currentTime := data.Data.(float64)
		msg := resource.ServerMessage{
			Data: resource.BuildPacket(opcode.Seek, currentTime),
			Target: resource.MessageTarget{
				IgnoredSessionIds: map[string]bool{c.Session.Id: true},
			},
		}

		roomId := c.Session.RoomId
		stateKey := fmt.Sprintf(constant.RoomStateFmt, roomId)

		err := h.app.DispatchRoom(roomId, msg)

		if err != nil {
			panic(err)
		}

		err = rdb.HSet(ctx, stateKey, "currentTime", currentTime).Err()

		if err != nil {
			panic(err)
		}
	}
}

func (h *Handlers) HandleSkip(data *resource.Packet, c *client.Client) {
	if !c.Session.HasPermission(permission.VIDEO_REMOTE) {
		return
	}

	h.nextItem(c.Context(), c.Session.RoomId)
}

func (h *Handlers) HandleVideoEnd(data *resource.Packet, c *client.Client) {
	roomId := c.Session.RoomId

	if roomId == 0 {
		return
	}

	videoId, ok := data.Data.(string)

	if !ok {
		return
	}

	ctx := c.Context()
	rdb := h.app.GetRedis()

	currentItemKey := fmt.Sprintf(constant.RoomCurrentItemFmt, roomId)
	currVideoId, err := rdb.HGet(ctx, currentItemKey, "id").Result()

	if err != nil {
		panic(err)
	}

	if currVideoId == videoId {
		usersKey := fmt.Sprintf(constant.RoomUsersFmt, roomId)
		ackKey := fmt.Sprintf(constant.RoomVideoEndAckFmt, roomId)

		pipe := rdb.Pipeline()

		pipe.SAdd(ctx, ackKey, c.Session.UserId)
		ackCountCmd := pipe.SCard(ctx, ackKey)
		totalCountCmd := pipe.SCard(ctx, usersKey)

		_, err = pipe.Exec(ctx)

		if err != nil {
			panic(err)
		}

		ackCount := ackCountCmd.Val()
		totalCount := totalCountCmd.Val()

		if ackCount >= totalCount / 2 {
			h.nextItem(h.app.Context(), roomId)
		}
	}
}


func (h *Handlers) nextItem(ctx context.Context, roomId model.RoomId) {
	item, err := h.popItem(ctx, roomId)

	if err != nil {
		if err == redis.Nil {
			item = nil
		} else {
			log.WithError(err).Error("Failed to play next queue item")
			return
		}
	}

	if item != nil {
		queueRemoveMsg := resource.ServerMessage{
			Data: resource.BuildPacket(opcode.QueueRemove, item.Id),
		}

		err = h.app.DispatchRoom(roomId, queueRemoveMsg)

		if err != nil {
			panic(err)
		}
	} else {
		rdb := h.app.GetRedis()
		currentItemKey := fmt.Sprintf(constant.RoomCurrentItemFmt, roomId)

		var exists int64
		exists, err = rdb.Exists(ctx, currentItemKey).Result()

		if err != nil || exists == 0 {
			return // don't skip if nothing is playing and nothing is in queue
		}
	}

	h.setCurrentItem(ctx, roomId, item)
}

func (h *Handlers) setCurrentItem(ctx context.Context, roomId model.RoomId, item *resource.MediaItem) {
	state := resource.PlayerState{
		IsPlaying: false,
		CurrentTime: 0,
		PlaybackStart: time.Now(),
	}

	setVideoMsg := resource.ServerMessage{
		Data: resource.BuildPacket(opcode.VideoSet, item),
	}

	err := h.app.DispatchRoom(roomId, setVideoMsg)

	if err != nil {
		panic(err)
	}

	err = h.dispatchState(roomId, &state)

	if err != nil {
		panic(err)
	}

	rdb := h.app.GetRedis()

	currentItemKey := fmt.Sprintf(constant.RoomCurrentItemFmt, roomId)
	stateKey := fmt.Sprintf(constant.RoomStateFmt, roomId)
	ackKey := fmt.Sprintf(constant.RoomVideoEndAckFmt, roomId)

	pipe := rdb.Pipeline()

	if item != nil {
		pipe.HSet(ctx, currentItemKey,
			"id", item.Id,
			"url", item.Url,
			"title", item.Title,
			"icon", item.Icon,
		)
	} else {
		pipe.Del(ctx, currentItemKey)
	}

	pipe.Del(ctx, ackKey)
	pipe.HSet(ctx, stateKey,
		"currentTime", state.CurrentTime,
		"playing", state.IsPlaying,
		"playbackStart", state.PlaybackStart,
	)

	_, err = pipe.Exec(ctx)

	if err != nil {
		panic(err)
	}
}

func (h *Handlers) getState(ctx context.Context, roomId model.RoomId) (*resource.PlayerState, error) {
	rdb := h.app.GetRedis()
	stateKey := fmt.Sprintf(constant.RoomStateFmt, roomId)

	vals, err := rdb.HGetAll(ctx, stateKey).Result()

	if err != nil {
		return nil, err
	}

	playbackStart, err := time.Parse(time.RFC3339Nano, vals["playbackStart"])

	if err != nil {
		return nil, err
	}

	currentTime, err := strconv.ParseFloat(vals["currentTime"], 64)

	if err != nil {
		return nil, err
	}

	state := resource.PlayerState{
		IsPlaying: vals["playing"] == "1",
		PlaybackStart: playbackStart,
		CurrentTime: currentTime,
	}

	if state.IsPlaying {
		timeDiff := time.Now().Sub(state.PlaybackStart).Seconds()
		state.CurrentTime += timeDiff
	}

	return &state, nil
}

func (h *Handlers) dispatchState(roomId model.RoomId, state *resource.PlayerState) error {
	stateMsg := resource.ServerMessage{
		Data: buildState(state),
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

	err = c.Write(buildState(state))

	if err != nil {
		return err
	}

	return nil
}