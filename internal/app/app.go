package app

import (
	"context"
	"github.com/go-pg/pg/v10"
	"github.com/go-redis/cache/v8"
	"github.com/go-redis/redis/v8"
	"github.com/sakuraapp/gateway/internal/config"
	"github.com/sakuraapp/gateway/internal/manager"
	"github.com/sakuraapp/gateway/internal/repository"
	"github.com/sakuraapp/gateway/pkg/util"
	"github.com/sakuraapp/shared/pkg/model"
	resource2 "github.com/sakuraapp/shared/pkg/resource"
)

// note: this file has to be in a package of its own because it can't be imported in most places (or it will cause a cyclic dependency)

type App interface {
	Context() context.Context
	NodeId() string
	GetConfig() *config.Config
	GetBuilder() *resource2.Builder
	GetCrawler() *util.Crawler
	GetJWT() *util.JWT
	GetDB() *pg.DB
	GetRepos() *repository.Repositories
	GetRedis() *redis.Client
	GetCache() *cache.Cache
	GetHandlerMgr() *manager.HandlerManager
	GetClientMgr() *manager.ClientManager
	GetSessionMgr() *manager.SessionManager
	GetRoomMgr() *manager.RoomManager
	Dispatch(msg resource2.ServerMessage) error
	DispatchLocal(msg resource2.ServerMessage) error
	DispatchRoom(roomId model.RoomId, msg resource2.ServerMessage) error
	DispatchRoomLocal(roomId model.RoomId, msg resource2.ServerMessage) error
}