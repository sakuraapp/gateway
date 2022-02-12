package repository

import (
	"context"
	"fmt"
	"github.com/go-pg/pg/v10"
	"github.com/go-redis/cache/v8"
	"github.com/sakuraapp/shared/pkg/constant"
	"github.com/sakuraapp/shared/pkg/model"
)

type RoomRepository struct {
	db *pg.DB
	cache *cache.Cache
}

func (r *RoomRepository) Get(ctx context.Context, id model.RoomId) (*model.Room, error) {
	room := new(model.Room)

	if err := r.cache.Once(&cache.Item{
		Ctx:   ctx,
		Key:   fmt.Sprintf(constant.RoomCacheFmt, id),
		Value: room,
		TTL:   constant.RoomCacheTTL,
		Do: func(item *cache.Item) (interface{}, error) {
			return r.fetch(room, id)
		},
	}); err != nil {
		return nil, err
	}

	return room, nil
}

func (r *RoomRepository) fetch(room *model.Room, id model.RoomId) (*model.Room, error) {
	err := r.db.Model(room).
		Relation("Owner").
		Where("room.id = ?", id).
		First()

	return room, err
}