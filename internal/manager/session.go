package manager

import (
	"github.com/sakuraapp/gateway/internal/client"
	"github.com/sakuraapp/shared/pkg/model"
	"sync"
)

type SessionMap = map[string]*client.Session      // map indexed by session id containing session pointers
type UserSessionMap = map[model.UserId]SessionMap // map indexed by user id containing each session for that user

type SessionManager struct {
	mu       sync.Mutex
	sessions UserSessionMap
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(UserSessionMap),
	}
}

func (s *SessionManager) Add(session *client.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	userId := session.UserId
	sessionId := session.Id

	if s.sessions[userId] == nil {
		s.sessions[userId] = SessionMap{sessionId: session}
	} else {
		s.sessions[userId][sessionId] = session
	}
}

func (s *SessionManager) Remove(session *client.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	userId := session.UserId
	sessions := s.sessions[userId]

	if sessions != nil {
		delete(sessions, session.Id)

		if len(sessions) == 0 {
			delete(s.sessions, userId)
		}
	}
}

func (s *SessionManager) GetByUserId(userId model.UserId) SessionMap {
	return s.sessions[userId]
}