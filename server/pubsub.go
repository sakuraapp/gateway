package server

import (
	"fmt"
	"github.com/sakuraapp/shared/constant"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
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
				fmt.Printf("PubSub Error: %v", err)
				continue
			}

			var msg resource.ServerMessage

			err = msgpack.Unmarshal([]byte(message.Payload), &msg)

			if err != nil {
				fmt.Printf("PubSub Deserialization Error: %v", err)
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
					fmt.Printf("Unable to locally dispatch PubSub Message: %+v", msg)
				}
			} else {
				var roomId model.RoomId
				_, err = fmt.Sscanf(message.Channel, constant.RoomFmt, &roomId)

				if err != nil {
					fmt.Printf("Invalid PubSub Message Channel: %v\nErr: %v", message.Channel, err)
					continue
				}

				err = s.DispatchRoomLocal(roomId, msg)

				if err != nil {
					fmt.Printf("Unable to handle PubSub Room Message: %+v", msg)
				}
			}
		}
	}()
}