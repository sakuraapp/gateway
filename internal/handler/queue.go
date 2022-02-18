package handler

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/sakuraapp/gateway/internal/client"
	"github.com/sakuraapp/gateway/internal/gateway"
	"github.com/sakuraapp/gateway/pkg/util"
	"github.com/sakuraapp/pubsub"
	"github.com/sakuraapp/shared/pkg/constant"
	"github.com/sakuraapp/shared/pkg/model"
	"github.com/sakuraapp/shared/pkg/resource"
	"github.com/sakuraapp/shared/pkg/resource/opcode"
	"github.com/sakuraapp/shared/pkg/resource/permission"
	log "github.com/sirupsen/logrus"
	"io"
	"net/url"
)

func (h *Handlers) HandleQueueAdd(data *resource.Packet, c *client.Client) gateway.Error {
	roomId := c.Session.RoomId

	if roomId == 0 || !c.Session.HasPermission(permission.QUEUE_ADD) {
		return nil
	}

	inputUrl := data.Data.(string)
	rawUrl := inputUrl
	u, err := url.Parse(rawUrl)

	if err != nil {
		return nil
	}

	switch util.GetDomain(u) {
	case "youtube.com":
		if u.Path == "/watch" {
			videoId := u.Query().Get("v")
			rawUrl = fmt.Sprintf("https://www.youtube.com/embed/%v", videoId)
		}
	}

	itemInfo, err := h.app.GetCrawler().Get(inputUrl)

	if err != nil && err != io.EOF {
		return gateway.NewError(gateway.ErrorCrawler, err)
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
	currentCmd := pipe.Exists(ctx, currentItemKey)

	_, err = pipe.Exec(ctx)

	if err != nil {
		return gateway.NewError(gateway.ErrorRedis, err)
	}

	if lenCmd.Val() > 0 || currentCmd.Val() == 1 {
		// something else is already playing
		pipe = rdb.Pipeline()

		pipe.HSet(ctx, queueItemsKey, item.Id, item)
		pipe.RPush(ctx, queueKey, item.Id)

		_, err = pipe.Exec(ctx)

		if err != nil {
			return gateway.NewError(gateway.ErrorRedis, err)
		}

		queueAddMessage := pubsub.Message{
			Data: resource.BuildPacket(opcode.QueueAdd, item),
		}

		err = h.app.DispatchRoom(roomId, &queueAddMessage)

		if err != nil {
			return gateway.NewError(gateway.ErrorDispatch, err)
		}
	} else {
		err = h.setCurrentItem(h.app.Context(), roomId, &item)

		if err != nil {
			return gateway.NewError(gateway.ErrorSetCurrentItem, err)
		}
	}

	return nil
}

func (h *Handlers) HandleQueueRemove(data *resource.Packet, c *client.Client) gateway.Error {
	roomId := c.Session.RoomId

	if roomId == 0 {
		return nil
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
			return gateway.NewError(gateway.ErrorRedis, err)
		}

		userId := c.Session.UserId

		if item.Author != userId {
			log.WithField("user_id", userId).Warn("Detected an attempt to remove a queue item without permission")
			return nil
		}
	}

	pipe := rdb.Pipeline()

	pipe.LRem(ctx, queueKey, 1, id)
	pipe.HDel(ctx, queueItemsKey, id)

	_, err := pipe.Exec(ctx)

	if err != nil {
		return gateway.NewError(gateway.ErrorRedis, err)
	}

	queueRemoveMessage := pubsub.Message{
		Data: resource.BuildPacket(opcode.QueueRemove, id),
	}

	err = h.app.DispatchRoom(roomId, &queueRemoveMessage)

	if err != nil {
		return gateway.NewError(gateway.ErrorDispatch, err)
	}

	return nil
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