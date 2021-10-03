package server

import (
	"fmt"
	"github.com/sakuraapp/shared/resource"
	"github.com/vmihailenco/msgpack/v5"
)

const gatewayFmt = "gateway.%v"
const roomFmt = "room.%v"
const broadcastChName = "gateway.broadcast"

func (s *Server) initPubsub() {
	nodeId := s.NodeId()
	rdb := s.rdb
	ctx := s.ctx

	chName := fmt.Sprintf(gatewayFmt, nodeId)

	pubsub := rdb.Subscribe(ctx, chName, broadcastChName)
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

			if ch == chName || ch == broadcastChName {
				s.DispatchLocal(msg)
			} else {
				var roomId int
				_, err := fmt.Sscanf(message.Channel, roomFmt, &roomId)

				if err != nil {
					fmt.Printf("Invalid PubSub Message Channel: %v\nErr: %v", message.Channel, err)
					continue
				}

			}
		}
	}()
}