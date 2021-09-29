package client

import (
	"github.com/google/uuid"
	"github.com/sakuraapp/shared/model"
	"gopkg.in/guregu/null.v4"
)

type Session struct {
	Id string `redis:"-"`
	UserId model.UserId
	RoomId null.Int
}

func NewSession(userId model.UserId) *Session {
	return &Session{
		Id: uuid.NewString(),
		UserId: userId,
	}
}