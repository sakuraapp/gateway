package server

import (
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/sakuraapp/shared/constant"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
)

func (s *Server) DispatchLocal(msg resource.ServerMessage) error {
	clientMgr := s.clients
	sessMgr := s.sessions

	clients := clientMgr.Clients()
	mu := clientMgr.Mutex()

	mu.Lock()
	defer mu.Unlock()

	ignoredSessions := msg.Target.IgnoredSessionIds

	switch msg.Type {
	case resource.BROADCAST_MESSAGE:
		for _, c := range clients {
			session := c.Session

			if session != nil {
				isIgnored := ignoredSessions[session.Id]

				if !isIgnored {
					err := c.Write(msg.Data)

					if err != nil {
						return err
					}
				}
			}
		}
	case resource.NORMAL_MESSAGE:
		for userId := range msg.Target.UserIds {
			sessionIds := sessMgr.GetByUserId(userId)

			for sessionId := range sessionIds {
				isIgnored := ignoredSessions[sessionId]
				c := clients[sessionId]

				if !isIgnored && c != nil {
					err := c.Write(msg.Data)

					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func (s *Server) Dispatch(msg resource.ServerMessage) error {
	msg.Origin = s.NodeId()

	if msg.Type == resource.BROADCAST_MESSAGE {
		err := s.DispatchLocal(msg)

		if err != nil {
			return err
		}

		return s.rdb.Publish(s.ctx, constant.BroadcastChName, msg).Err()
	} else if msg.Type == resource.NORMAL_MESSAGE {
		pipe := s.rdb.Pipeline()
		locNodeId := s.NodeId()

		for userId := range msg.Target.UserIds {
			pipe.SMembers(s.ctx, fmt.Sprintf(constant.UserSessionsFmt, userId))
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
				sessionKey = fmt.Sprintf(constant.SessionFmt, session)
				pipe.HGet(s.ctx, sessionKey, "node_id")
			}
		}

		results, err = pipe.Exec(s.ctx)

		if err != nil {
			return err
		}

		bytes, err := msgpack.Marshal(msg)

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
					nodeKey = fmt.Sprintf(constant.GatewayFmt, nodeId)
					pipe.Publish(s.ctx, nodeKey, bytes)
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

func (s *Server) DispatchRoomLocal(roomId model.RoomId, msg resource.ServerMessage) error {
	r := s.rooms.Get(roomId)
	var err error

	if r != nil {
		mu := r.Mutex()
		mu.Lock()
		defer mu.Unlock()

		for c := range r.Clients() {
			if !msg.Target.IgnoredSessionIds[c.Session.Id] {
				err = c.Write(msg.Data)

				if err != nil {
					log.Warnf("Failed to write to session %v\n", c.Session.Id)
					return err
				}
			}
		}
	}

	return nil
}

func (s *Server) DispatchRoom(roomId model.RoomId, msg resource.ServerMessage) error {
	msg.Origin = s.NodeId()
	err := s.DispatchRoomLocal(roomId, msg)

	if err != nil {
		return err
	}

	roomKey := fmt.Sprintf(constant.RoomFmt, roomId)
	bytes, err := msgpack.Marshal(msg)

	if err != nil {
		return err
	}

	return s.rdb.Publish(s.ctx, roomKey, bytes).Err()
}