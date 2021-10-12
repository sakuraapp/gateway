package internal

import (
	"context"
	"github.com/go-pg/pg/v10"
	"github.com/go-redis/cache/v8"
	"github.com/go-redis/redis/v8"
	"github.com/sakuraapp/gateway/config"
	"github.com/sakuraapp/gateway/manager"
	"github.com/sakuraapp/gateway/repository"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
)

type App interface {
	Context() context.Context
	NodeId() string
	GetConfig() *config.Config
	GetJWT() *JWT
	GetDB() *pg.DB
	GetRepos() *repository.Repositories
	GetRedis() *redis.Client
	GetCache() *cache.Cache
	GetHandlerMgr() *manager.HandlerManager
	GetClientMgr() *manager.ClientManager
	GetRoomMgr() *manager.RoomManager
	Dispatch(msg resource.ServerMessage) error
	DispatchLocal(msg resource.ServerMessage) error
	DispatchRoom(roomId model.RoomId, msg resource.ServerMessage) error
	DispatchRoomLocal(roomId model.RoomId, msg resource.ServerMessage) error
}