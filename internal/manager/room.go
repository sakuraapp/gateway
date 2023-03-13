package manager

import (
	"context"
	"github.com/sakuraapp/gateway/internal/client"
	dispatcher "github.com/sakuraapp/shared/pkg/dispatcher/gateway"
	"github.com/sakuraapp/shared/pkg/model"
	"github.com/sakuraapp/shared/pkg/resource/permission"
	log "github.com/sirupsen/logrus"
	"sync"
)

type Room struct {
	id      model.RoomId
	mu      sync.Mutex
	clients map[*client.Client]bool
}

func (r *Room) Mutex() *sync.Mutex {
	return &r.mu
}

func (r *Room) Clients() map[*client.Client]bool {
	return r.clients
}

func (r *Room) NumClients() int {
	return len(r.clients)
}

func (r *Room) Add(c *client.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clients[c] = true
}

func (r *Room) Remove(c *client.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients, c)
}

func (r *Room) Dispatch(msg *dispatcher.Message) {
	var err error

	var ignoredSessionId string
	var perms permission.Permission

	if msg.Filters != nil {
		for filter, value := range msg.Filters {
			switch filter {
			case dispatcher.MessageFilterIgnoredSession:
				ignoredSessionId = value.(string)
			case dispatcher.MessageFilterPermissions:
				perms = value.(permission.Permission)
			}
		}
	}

	for c := range r.clients {
		if ignoredSessionId == c.Session.Id {
			continue
		}

		if perms > 0 && !c.Session.HasPermission(perms) {
			continue
		}

		err = c.Write(msg.Payload)

		if err != nil {
			log.WithError(err).Error("Failed to write message to client")
		}
	}
}

type RoomManager struct {
	mu     sync.Mutex
	subMgr *dispatcher.SubscriptionManager
	rooms  map[model.RoomId]*Room
}

func NewRoomManager(subMgr *dispatcher.SubscriptionManager) *RoomManager {
	return &RoomManager{
		subMgr: subMgr,
		rooms:  map[model.RoomId]*Room{},
	}
}

func (m *RoomManager) Get(roomId model.RoomId) *Room {
	return m.rooms[roomId]
}

func (m *RoomManager) Create(ctx context.Context, roomId model.RoomId) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	room := &Room{
		id:      roomId,
		clients: map[*client.Client]bool{},
	}

	m.rooms[roomId] = room

	roomTopic := dispatcher.NewRoomTarget(roomId).Build()
	err := m.subMgr.Add(ctx, roomTopic, room)

	if err != nil {
		return nil, err
	}

	return room, nil
}

func (m *RoomManager) Delete(ctx context.Context, roomId model.RoomId) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	room := m.rooms[roomId]
	roomTopic := dispatcher.NewRoomTarget(roomId).Build()

	delete(m.rooms, roomId)

	return m.subMgr.Remove(ctx, roomTopic, room)
}
