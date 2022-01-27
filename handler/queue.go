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
	log "github.com/sirupsen/logrus"
	"io"
	"net/url"
)

func (h *Handlers) HandleQueueAdd(data *resource.Packet, c *client.Client) {
	roomId := c.Session.RoomId

	if roomId == 0 || !c.Session.HasPermission(permission.QUEUE_ADD) {
		return
	}

	inputUrl := data.Data.(string)
	rawUrl := inputUrl
	u, err := url.Parse(rawUrl)

	if err != nil {
		return
	}

	switch internal.GetDomain(u) {
	case "youtube.com":
		if u.Path == "/watch" {
			videoId := u.Query().Get("v")
			rawUrl = fmt.Sprintf("https://www.youtube.com/embed/%v", videoId)
		}
	}

	itemInfo, err := h.app.GetCrawler().Get(inputUrl)

	if err != nil && err != io.EOF {
		panic(err)
	}

	// note that empty titles & icons are handled client-side

	itemInfo.Url = rawUrl
	item := resource.MediaItem{
		Id: uuid.NewString(),
		Author: c.Session.UserId,
		MediaItemInfo: itemInfo,
	}

	queueKey := fmt.Sprintf(constant.RoomQueueFmt, roomId)
	currentItemKey := fmt.Sprintf(constant.RoomCurrentItemFmt, roomId)
	queueItemsKey := fmt.Sprintf(constant.RoomQueueItemsFmt, roomId)

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
		pipe = rdb.Pipeline()

		pipe.HSet(ctx, queueItemsKey, item.Id, item)
		pipe.RPush(ctx, queueKey, item.Id)

		_, err = pipe.Exec(ctx)

		if err != nil {
			panic(err)
		}

		queueAddMessage := resource.ServerMessage{
			Data: resource.BuildPacket(opcode.QueueAdd, item),
		}

		err = h.app.DispatchRoom(roomId, queueAddMessage)

		if err != nil {
			panic(err)
		}
	} else {
		h.setCurrentItem(h.app.Context(), roomId, &item)
	}
}

func (h *Handlers) HandleQueueRemove(data *resource.Packet, c *client.Client) {
	roomId := c.Session.RoomId

	if roomId == 0 {
		return
	}

	queueKey := fmt.Sprintf(constant.RoomQueueFmt, roomId)
	queueItemsKey := fmt.Sprintf(constant.RoomQueueItemsFmt, roomId)

	ctx := c.Context()
	rdb := h.app.GetRedis()

	id := data.Data.(string)

	if !c.Session.HasPermission(permission.QUEUE_EDIT) {
		var item resource.MediaItem
		
		err := rdb.HGet(ctx, queueItemsKey, id).Scan(&item)

		if err != nil {
			panic(err)
		}

		userId := c.Session.UserId

		if item.Author != userId {
			log.WithField("user_id", userId).Warn("Detected an attempt to remove a queue item without permission")
			return
		}
	}

	pipe := rdb.Pipeline()

	pipe.LRem(ctx, queueKey, 1, id)
	pipe.HDel(ctx, queueItemsKey, id)

	_, err := pipe.Exec(ctx)

	if err != nil {
		panic(err)
	}

	queueRemoveMessage := resource.ServerMessage{
		Data: resource.BuildPacket(opcode.QueueRemove, id),
	}

	err = h.app.DispatchRoom(roomId, queueRemoveMessage)

	if err != nil {
		panic(err)
	}
}

func (h *Handlers) popItem(ctx context.Context, roomId model.RoomId) (*resource.MediaItem, error) {
	queueKey := fmt.Sprintf(constant.RoomQueueFmt, roomId)
	itemsKey := fmt.Sprintf(constant.RoomQueueItemsFmt, roomId)

	rdb := h.app.GetRedis()

	// pop id from the front of the queue
	id, err := rdb.LPop(ctx, queueKey).Result()

	if err != nil {
		return nil, err
	}

	pipe := rdb.Pipeline()

	getCmd := pipe.HGet(ctx, itemsKey, id) // get the item info of the popped id
	pipe.HDel(ctx, itemsKey, id) // delete the item's info

	_, err = pipe.Exec(ctx)

	if err != nil {
		return nil, err
	}

	var item resource.MediaItem

	err = getCmd.Scan(&item)

	if err != nil {
		return nil, err
	}

	return &item, nil
}