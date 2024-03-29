package handler

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/sakuraapp/gateway/internal/client"
	"github.com/sakuraapp/gateway/internal/gateway"
	"github.com/sakuraapp/pubsub"
	"github.com/sakuraapp/shared/pkg/constant"
	dispatcher "github.com/sakuraapp/shared/pkg/dispatcher/gateway"
	"github.com/sakuraapp/shared/pkg/model"
	"github.com/sakuraapp/shared/pkg/resource"
	"github.com/sakuraapp/shared/pkg/resource/opcode"
	"github.com/sakuraapp/shared/pkg/resource/permission"
	"strconv"
	"time"
)

func buildState(state *resource.PlayerState) resource.Packet {
	data := map[string]interface{}{
		"playing":       state.IsPlaying,
		"currentTime":   state.CurrentTime,
		"playbackStart": state.PlaybackStart,
	}

	return resource.BuildPacket(opcode.PlayerState, data)
}

func (h *Handlers) HandleSetPlayerState(data *resource.Packet, c *client.Client) gateway.Error {
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
			IsPlaying:     m["playing"].(bool),
			CurrentTime:   m["currentTime"].(float64),
			PlaybackStart: t,
		}

		roomId := c.Session.RoomId
		stateKey := fmt.Sprintf(constant.RoomStateFmt, roomId)

		err := h.app.DispatchTo(dispatcher.NewRoomTarget(roomId), &dispatcher.Message{
			Payload: resource.BuildPacket(opcode.PlayerState, state),
			Filters: dispatcher.NewFilterMap().WithIgnoredSession(c.Session.Id),
		})

		if err != nil {
			return gateway.NewError(gateway.ErrorDispatch, err)
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
			return gateway.NewError(gateway.ErrorRedis, err)
		}
	}

	return nil
}

func (h *Handlers) HandleSeek(data *resource.Packet, c *client.Client) gateway.Error {
	if c.Session.HasPermission(permission.VIDEO_REMOTE) {
		ctx := h.app.Context()
		rdb := h.app.GetRedis()

		currentTime := data.Data.(float64)

		roomId := c.Session.RoomId
		stateKey := fmt.Sprintf(constant.RoomStateFmt, roomId)

		err := h.app.DispatchTo(dispatcher.NewRoomTarget(roomId), &dispatcher.Message{
			Payload: resource.BuildPacket(opcode.Seek, currentTime),
			Filters: dispatcher.NewFilterMap().WithIgnoredSession(c.Session.Id),
		})

		if err != nil {
			return gateway.NewError(gateway.ErrorDispatch, err)
		}

		err = rdb.HSet(ctx, stateKey, "currentTime", currentTime).Err()

		if err != nil {
			return gateway.NewError(gateway.ErrorRedis, err)
		}
	}

	return nil
}

func (h *Handlers) HandleSkip(data *resource.Packet, c *client.Client) gateway.Error {
	if !c.Session.HasPermission(permission.VIDEO_REMOTE) {
		return nil
	}

	err := h.nextItem(c.Context(), c.Session.RoomId)

	if err != nil {
		return gateway.NewError(gateway.ErrorNextItem, err) // todo: rethink this and whether nextItem should return a regular error or a gateway error
	}

	return nil
}

func (h *Handlers) HandleVideoEnd(data *resource.Packet, c *client.Client) gateway.Error {
	roomId := c.Session.RoomId

	if roomId == 0 {
		return nil
	}

	videoId, ok := data.Data.(string)

	if !ok {
		return nil
	}

	ctx := c.Context()
	rdb := h.app.GetRedis()

	currentItemKey := fmt.Sprintf(constant.RoomCurrentItemFmt, roomId)
	currVideoId, err := rdb.HGet(ctx, currentItemKey, "id").Result()

	if err != nil {
		if err == redis.Nil {
			return nil
		} else {
			return gateway.NewError(gateway.ErrorRedis, err)
		}
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
			return gateway.NewError(gateway.ErrorRedis, err)
		}

		ackCount := ackCountCmd.Val()
		totalCount := totalCountCmd.Val()

		if ackCount >= totalCount/2 {
			err = h.nextItem(h.app.Context(), roomId)

			if err != nil {
				return gateway.NewError(gateway.ErrorNextItem, err)
			}
		}
	}

	return nil
}

func (h *Handlers) nextItem(ctx context.Context, roomId model.RoomId) error {
	item, err := h.popItem(ctx, roomId)

	if err != nil {
		if err == redis.Nil {
			item = nil
		} else {
			return err
		}
	}

	if item != nil {
		packet := resource.BuildPacket(opcode.QueueRemove, item.Id)
		err = h.app.DispatchTo(dispatcher.NewRoomTarget(roomId), dispatcher.NewMessage(packet))

		if err != nil {
			return err
		}
	} else {
		rdb := h.app.GetRedis()
		currentItemKey := fmt.Sprintf(constant.RoomCurrentItemFmt, roomId)

		var exists int64
		exists, err = rdb.Exists(ctx, currentItemKey).Result()

		if err != nil || exists == 0 {
			return nil // don't skip if nothing is playing and nothing is in queue
		}
	}

	return h.SetCurrentItem(ctx, roomId, item)
}

func (h *Handlers) SetCurrentItem(ctx context.Context, roomId model.RoomId, item *resource.MediaItem) error {
	state := resource.PlayerState{
		IsPlaying:     false,
		CurrentTime:   0,
		PlaybackStart: time.Now(),
	}

	packet := resource.BuildPacket(opcode.VideoSet, item)
	err := h.app.DispatchTo(dispatcher.NewRoomTarget(roomId), pubsub.NewMessage(packet))

	if err != nil {
		return err
	}

	err = h.dispatchState(roomId, &state)

	if err != nil {
		return err
	}

	rdb := h.app.GetRedis()

	currentItemKey := fmt.Sprintf(constant.RoomCurrentItemFmt, roomId)
	stateKey := fmt.Sprintf(constant.RoomStateFmt, roomId)
	ackKey := fmt.Sprintf(constant.RoomVideoEndAckFmt, roomId)

	pipe := rdb.Pipeline()

	if item != nil {
		pipe.HSet(ctx, currentItemKey,
			"id", item.Id,
			"type", (int)(item.Type),
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

	return err
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
		IsPlaying:     vals["playing"] == "1",
		PlaybackStart: playbackStart,
		CurrentTime:   currentTime,
	}

	if state.IsPlaying {
		timeDiff := time.Now().Sub(state.PlaybackStart).Seconds()
		state.CurrentTime += timeDiff
	}

	return &state, nil
}

func (h *Handlers) dispatchState(roomId model.RoomId, state *resource.PlayerState) error {
	return h.app.DispatchTo(dispatcher.NewRoomTarget(roomId), dispatcher.NewMessage(buildState(state)))
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
