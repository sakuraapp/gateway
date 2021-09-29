package manager

import (
	"github.com/sakuraapp/gateway/client"
	"sync"
)

type ClientManager struct {
	mu sync.Mutex
	clients map[*client.Client]bool
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		clients: map[*client.Client]bool{},
	}
}

func (m *ClientManager) Add(c *client.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clients[c] = true
}

func (m *ClientManager) Remove(c *client.Client)  {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.clients, c)
}