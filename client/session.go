package client

import (
	"github.com/google/uuid"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource/permission"
	"time"
)

const SessionExpiryDuration = 15 * time.Minute

type Session struct {
	Id string `json:"id" redis:"id"`
	UserId model.UserId `json:"user_id" redis:"user_id"`
	RoomId model.RoomId `json:"room_id" redis:"room_id"`
	NodeId string `json:"node_id" redis:"node_id"`
	Permissions permission.Permission `json:"-" redis:"-"`
}

func (s *Session) HasPermission(perm permission.Permission) bool {
	return (s.Permissions & perm) > 0
}

func (s *Session) AddPermission(perm permission.Permission) {
	s.Permissions |= perm
}

func (s *Session) RemovePermission(perm permission.Permission) {
	if s.HasPermission(perm) {
		s.Permissions ^= perm
	}
}

func NewSession(userId model.UserId, nodeId string) *Session {
	return &Session{
		Id: uuid.NewString(),
		UserId: userId,
		NodeId: nodeId,
	}
}