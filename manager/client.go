package manager

import (
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/sakuraapp/gateway/client"
	"sync"
	"time"
)

const (
	keepAliveTime = time.Second * 5
	KeepAliveTimeout = keepAliveTime + time.Second * 3
)

type ClientManager struct {
	stopCh chan struct{}
	mu sync.Mutex
	clients map[*client.Client]bool
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		stopCh: make(chan struct{}),
		clients: map[*client.Client]bool{},
	}
}

func (m *ClientManager) Mutex() *sync.Mutex {
	return &m.mu
}

func (m *ClientManager) Clients() map[*client.Client]bool {
	return m.clients
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

func (m *ClientManager) StartTicker() {
	ticker := time.NewTicker(time.Second)

	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			func() {
				m.mu.Lock()
				defer m.mu.Unlock()

				mustActive := time.Now().Add(-keepAliveTime)
				nPing := 0

				for c := range m.clients {
					lastActive := c.LastActive

					if lastActive.Before(mustActive) {
						err := c.Conn().WriteMessage(websocket.PingMessage, nil)

						if err != nil {
							continue
						}

						nPing++
					}
				}
			}()
		}
	}
}

func (m *ClientManager) StopTicker() {
	close(m.stopCh)
}