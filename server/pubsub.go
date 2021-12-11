package server

import (
	"fmt"
	"github.com/sakuraapp/shared/constant"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
)

func (s *Server) initPubsub() {
	nodeId := s.NodeId()
	rdb := s.rdb
	ctx := s.ctx

	chName := fmt.Sprintf(constant.GatewayFmt, nodeId)

	pubsub := rdb.Subscribe(ctx, chName, constant.BroadcastChName)
	s.pubsub = pubsub

	go func() {
		for {
			message, err := pubsub.ReceiveMessage(ctx)

			if err != nil {
				log.Errorf("PubSub Error: %v", err)
				continue
			}

			var msg resource.ServerMessage

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
				err = s.DispatchLocal(msg)

				if err != nil {
					log.Errorf("Unable to locally dispatch PubSub Message: %+v", msg)
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

				err = s.DispatchRoomLocal(roomId, msg)

				if err != nil {
					log.WithError(err).Error("Unable to handle PubSub Room Message")
				}
			}
		}
	}()
}