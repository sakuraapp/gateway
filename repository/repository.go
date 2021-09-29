package repository

import (
	"github.com/go-pg/pg/v10"
	"github.com/go-redis/cache/v8"
)

type Repositories struct {
	User *UserRepository
}

func Init(db *pg.DB, cache *cache.Cache) *Repositories {
	return &Repositories{
		User: &UserRepository{
			db: db,
			cache: cache,
		},
	}
}