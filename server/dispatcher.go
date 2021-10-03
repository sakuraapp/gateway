package server

import (
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/gateway/pkg"
	"github.com/sakuraapp/shared/resource"
)

func (s *Server) DispatchLocal(msg resource.ServerMessage) {
	mgr := s.clients
	clients := mgr.Clients()
	mu := mgr.Mutex()

	mu.Lock()
	defer mu.Unlock()

	for client := range clients {
		session := client.Session

		if session != nil {
			isBroadcast := msg.Type == resource.BROADCAST_MESSAGE
			isTargeted := msg.Type == resource.NORMAL_MESSAGE && msg.Target.UserIds[session.UserId]
			isIgnored := msg.Target.IgnoredSessionIds[session.Id]

			if !isIgnored && (isBroadcast || isTargeted) {
				err := client.Write(msg.Data)

				if err != nil {
					panic(err)
				}
			}
		}
	}
}

func (s *Server) Dispatch(msg resource.ServerMessage) error {
	if msg.Type == resource.BROADCAST_MESSAGE {
		s.DispatchLocal(msg)
		s.rdb.Publish(s.ctx, broadcastChName, msg)
	} else if msg.Type == resource.NORMAL_MESSAGE {
		pipe := s.rdb.Pipeline()
		locNodeId := s.NodeId()

		for userId := range msg.Target.UserIds {
			pipe.SMembers(s.ctx, fmt.Sprintf(pkg.UserSessionsFmt, userId))
		}

		results, err := pipe.Exec(s.ctx)

		if err != nil {
			return err
		}

		pipe = s.rdb.Pipeline()
		var sessionKey string

		for _, result := range results {
			sessions := result.(*redis.StringSliceCmd).Val()

			for _, session := range sessions {
				sessionKey = fmt.Sprintf(client.SessionFmt, session)
				pipe.HGet(s.ctx, sessionKey, "node_id")
			}
		}

		results, err = pipe.Exec(s.ctx)

		if err != nil {
			return err
		}

		pipe = s.rdb.Pipeline()
		nodes := map[string]bool{}
		var nodeKey string

		for _, result := range results {
			nodeId := result.(*redis.StringCmd).Val()
			
			if !nodes[nodeId] {
				nodes[nodeId] = true
				
				if nodeId == locNodeId {
					s.DispatchLocal(msg)
				} else {
					nodeKey = fmt.Sprintf(gatewayFmt, nodeId)
					pipe.Publish(s.ctx, nodeKey, msg)
				}
			}
		}

		_, err = pipe.Exec(s.ctx)

		if err != nil {
			return err
		}
	}

	return nil
}