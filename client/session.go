package client

import (
	"github.com/google/uuid"
	"github.com/sakuraapp/shared/model"
	"time"
)

const SessionFmt = "session.%v"
const SessionExpiryDuration = 15 * time.Minute

type Session struct {
	Id string `json:"id" redis:"id"`
	UserId model.UserId `json:"user_id" redis:"user_id"`
	RoomId model.RoomId `json:"room_id" redis:"room_id"`
	NodeId string `json:"node_id" redis:"node_id"`
}

func NewSession(userId model.UserId, nodeId string) *Session {
	return &Session{
		Id: uuid.NewString(),
		UserId: userId,
		NodeId: nodeId,
	}
}