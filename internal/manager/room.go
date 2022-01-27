package manager

import (
	"fmt"
	"github.com/sakuraapp/gateway/internal/client"
	"github.com/sakuraapp/shared/constant"
	"github.com/sakuraapp/shared/model"
	"sync"
)

type Room struct {
	id model.RoomId
	mu sync.Mutex
	subMgr *SubscriptionManager
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

func (r *Room) Add(c *client.Client) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clients[c] = true
	roomKey := fmt.Sprintf(constant.RoomFmt, r.id)

	return r.subMgr.Add(roomKey)
}

func (r *Room) Remove(c *client.Client) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients, c)

	roomKey := fmt.Sprintf(constant.RoomFmt, r.id)

	return r.subMgr.Remove(roomKey)
}

func newRoom(subMgr *SubscriptionManager, id model.RoomId) *Room {
	return &Room{
		id: id,
		subMgr: subMgr,
		clients: map[*client.Client]bool{},
	}
}

type RoomManager struct {
	mu sync.Mutex
	subMgr *SubscriptionManager
	rooms map[model.RoomId]*Room
}

func NewRoomManager(subMgr *SubscriptionManager) *RoomManager {
	return &RoomManager{
		subMgr: subMgr,
		rooms: map[model.RoomId]*Room{},
	}
}

func (m *RoomManager) Get(roomId model.RoomId) *Room {
	return m.rooms[roomId]
}

func (m *RoomManager) Create(roomId model.RoomId) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rooms[roomId] = newRoom(m.subMgr, roomId)

	return m.rooms[roomId]
}

func (m *RoomManager) Delete(roomId model.RoomId) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.rooms, roomId)
}