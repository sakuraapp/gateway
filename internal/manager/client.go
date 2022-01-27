package manager

import (
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/sakuraapp/gateway/internal/client"
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
	clients map[string]*client.Client
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		stopCh: make(chan struct{}),
		clients: map[string]*client.Client{},
	}
}

func (m *ClientManager) Mutex() *sync.Mutex {
	return &m.mu
}

func (m *ClientManager) Clients() map[string]*client.Client {
	return m.clients
}

func (m *ClientManager) Add(c *client.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clients[c.Session.Id] = c
}

func (m *ClientManager) Remove(c *client.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.clients, c.Session.Id)
}

func (m *ClientManager) Get(sessionId string) *client.Client {
	return m.clients[sessionId]
}

func (m *ClientManager) UpdateSession(c *client.Client, session *client.Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.clients, c.Session.Id)

	c.Session = session
	m.clients[session.Id] = c
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

				for _, c := range m.clients {
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