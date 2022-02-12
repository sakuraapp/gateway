package server

import (
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/sakuraapp/shared/pkg/constant"
	"github.com/sakuraapp/shared/pkg/model"
	"github.com/sakuraapp/shared/pkg/resource"
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
	roomId := msg.Target.RoomId

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
		for _, userId := range msg.Target.UserIds {
			sessions := sessMgr.GetByUserId(userId)

			for _, session := range sessions {
				isIgnored := ignoredSessions[session.Id]
				c := clients[session.Id]

				if roomId != 0 && roomId != session.RoomId {
					continue
				}

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

	switch msg.Type {
	case resource.BROADCAST_MESSAGE:
		err := s.DispatchLocal(msg)

		if err != nil {
			return err
		}

		return s.rdb.Publish(s.ctx, constant.BroadcastChName, msg).Err()
	case resource.NORMAL_MESSAGE:
		fallthrough
	case resource.SERVER_MESSAGE:
		// note: both normal and server messages can be dispatched toward certain users
		// todo: re-investigate how server messages should be targeted

		pipe := s.rdb.Pipeline()
		locNodeId := s.NodeId()
		roomId := msg.Target.RoomId

		for _, userId := range msg.Target.UserIds {
			if roomId == 0 {
				pipe.SMembers(s.ctx, fmt.Sprintf(constant.UserSessionsFmt, userId))
			} else {
				pipe.SMembers(s.ctx, fmt.Sprintf(constant.RoomUserSessionsFmt, roomId, userId))
			}
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

		if err != nil && err != redis.Nil {
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
			cmd := result.(*redis.StringCmd)

			if cmd.Err() == redis.Nil {
				continue
			}

			nodeId := cmd.Val()
			
			if !nodes[nodeId] {
				nodes[nodeId] = true
				
				if nodeId == locNodeId {
					if msg.Type == resource.SERVER_MESSAGE {
						s.handlers.HandleServer(&msg)
					} else {
						s.DispatchLocal(msg)
					}
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
	perms := msg.Target.Permissions
	r := s.rooms.Get(roomId)
	var err error

	if r != nil {
		mu := r.Mutex()
		mu.Lock()
		defer mu.Unlock()

		for c := range r.Clients() {
			if perms > 0 && c.Session.Roles != nil && !c.Session.HasPermission(perms) {
				continue
			}

			if !msg.Target.IgnoredSessionIds[c.Session.Id] {
				err = c.Write(msg.Data)

				if err != nil {
					log.
						WithField("session_id", c.Session.Id).
						WithError(err).
						Error("Failed to write to client")

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