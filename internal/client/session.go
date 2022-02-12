package client

import (
	"github.com/google/uuid"
	model2 "github.com/sakuraapp/shared/pkg/model"
	"github.com/sakuraapp/shared/pkg/resource/permission"
	"github.com/sakuraapp/shared/pkg/resource/role"
	"time"
)

const SessionExpiryDuration = 15 * time.Minute

type Session struct {
	Id     string        `json:"id" redis:"id"`
	UserId model2.UserId `json:"user_id" redis:"user_id"`
	RoomId model2.RoomId `json:"room_id" redis:"room_id"`
	NodeId string        `json:"node_id" redis:"node_id"`
	Roles  *role.Manager `json:"-" redis:"-"`
}

func (s *Session) HasPermission(perm permission.Permission) bool {
	return s.Roles.HasPermission(perm)
}

func NewSession(userId model2.UserId, nodeId string) *Session {
	return &Session{
		Id: uuid.NewString(),
		UserId: userId,
		NodeId: nodeId,
	}
}