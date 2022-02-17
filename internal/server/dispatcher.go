package server

import (
	"github.com/sakuraapp/shared/pkg/model"
	"github.com/sakuraapp/shared/pkg/resource"
	"github.com/sakuraapp/shared/pkg/resource/permission"
	log "github.com/sirupsen/logrus"
)

type GatewayDispatcher struct {
	server *Server
}

func (d *GatewayDispatcher) HandleServerMessage(msg *resource.ServerMessage) {
	d.server.handlers.HandleServer(msg)
}

func (d *GatewayDispatcher) DispatchLocal(msg *resource.ServerMessage) error {
	clientMgr := d.server.clients
	sessMgr := d.server.sessions

	clients := clientMgr.Clients()
	mu := clientMgr.Mutex()

	mu.Lock()
	defer mu.Unlock()

	var ignoredSessions map[string]bool
	var roomId model.RoomId

	if msg.Target != nil {
		ignoredSessions = msg.Target.IgnoredSessionIds
		roomId = msg.Target.RoomId
	}

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

func (d *GatewayDispatcher) DispatchRoomLocal(roomId model.RoomId, msg *resource.ServerMessage) error {
	var perms permission.Permission
	var ignoredSessions map[string]bool

	if msg.Target != nil {
		perms = msg.Target.Permissions
		ignoredSessions = msg.Target.IgnoredSessionIds
	}

	r := d.server.rooms.Get(roomId)
	var err error

	if r != nil {
		mu := r.Mutex()
		mu.Lock()
		defer mu.Unlock()

		for c := range r.Clients() {
			if perms > 0 && c.Session.Roles != nil && !c.Session.HasPermission(perms) {
				continue
			}

			if !ignoredSessions[c.Session.Id] {
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