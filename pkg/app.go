package pkg

import (
	"context"
	"github.com/go-pg/pg/v10"
	"github.com/go-redis/cache/v8"
	"github.com/go-redis/redis/v8"
	"github.com/sakuraapp/gateway/config"
	"github.com/sakuraapp/gateway/manager"
	"github.com/sakuraapp/gateway/repository"
)

type App interface {
	Context() context.Context
	GetConfig() *config.Config
	GetJWT() *JWT
	GetDB() *pg.DB
	GetRepos() *repository.Repositories
	GetRedis() *redis.Client
	GetCache() *cache.Cache
	GetHandlerMgr() *manager.HandlerManager
	GetClientMgr() *manager.ClientManager
}