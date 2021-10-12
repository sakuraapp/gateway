package repository

import (
	"github.com/go-pg/pg/v10"
	"github.com/sakuraapp/shared/model"
)

type RoomRepository struct {
	db *pg.DB
}

func (r *RoomRepository) Get(id model.RoomId) (*model.Room, error) {
	room := new(model.Room)
	err := r.db.Model(room).
		Relation("Owner").
		Where("room.id = ?", id).
		First()

	return room, err
}