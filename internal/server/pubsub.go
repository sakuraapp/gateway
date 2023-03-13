package server

import (
	"github.com/sakuraapp/shared/pkg/dispatcher/gateway"
	dispatcher "github.com/sakuraapp/shared/pkg/dispatcher/gateway"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
)

func (s *Server) initPubsub() {
	rdb := s.rdb
	ctx := s.ctx

	ps := rdb.Subscribe(ctx)

	s.pubsub = ps

	go func() {
		for {
			message, err := ps.ReceiveMessage(ctx)

			if err != nil {
				log.Errorf("PubSub Error: %v", err)
				continue
			}

			s.taskPool.Go(func() {
				var msg dispatcher.Message

				err = msgpack.Unmarshal([]byte(message.Payload), &msg)

				if err != nil {
					log.WithError(err).Error("PubSub Deserialization Error")
					return
				}

				ch := message.Channel

				log.WithField("channel", ch).Debugf("Incoming PubSub Message: %+v", msg)

				if msg.Filters[gateway.MessageFilterType] == gateway.ServerMessage {
					s.handlerMgr.HandleServer(&msg)
				} else {
					s.subscriptionMgr.Dispatch(ch, &msg)
				}
			})
		}
	}()
}
