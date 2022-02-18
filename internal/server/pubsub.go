package server

import (
	"fmt"
	"github.com/sakuraapp/pubsub"
	"github.com/sakuraapp/shared/pkg/constant"
	"github.com/sakuraapp/shared/pkg/model"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
)

func (s *Server) initPubsub() {
	nodeId := s.NodeId()
	rdb := s.rdb
	ctx := s.ctx

	chName := fmt.Sprintf(constant.GatewayFmt, nodeId)

	ps := rdb.Subscribe(ctx, chName, constant.BroadcastChName)
	s.pubsub = ps

	go func() {
		for {
			message, err := ps.ReceiveMessage(ctx)

			if err != nil {
				log.Errorf("PubSub Error: %v", err)
				continue
			}

			var msg pubsub.Message

			err = msgpack.Unmarshal([]byte(message.Payload), &msg)

			if err != nil {
				log.Errorf("PubSub Deserialization Error: %v", err)
				continue
			}

			// ignore own messages
			if msg.Origin == nodeId {
				continue
			}

			ch := message.Channel

			if ch == chName || ch == constant.BroadcastChName {
				if msg.Type == pubsub.ServerMessage {
					s.taskPool.Go(func() {
						s.handlers.HandleServer(&msg)
					})
				} else {
					err = s.DispatchLocal(&msg)

					if err != nil {
						log.Errorf("Unable to locally dispatch PubSub Message: %+v", msg)
					}
				}
			} else {
				var roomId model.RoomId
				_, err = fmt.Sscanf(message.Channel, constant.RoomFmt, &roomId)

				if err != nil {
					log.
						WithField("channel", message.Channel).
						WithError(err).
						Error("Failed to parse PubSub Message Channel")

					continue
				}

				log.WithField("room_id", roomId).Debugf("Incoming Room Message: %+v", msg)

				err = s.DispatchRoomLocal(roomId, &msg)

				if err != nil {
					log.WithError(err).Error("Unable to handle PubSub Room Message")
				}
			}
		}
	}()
}