package gateway

import (
	"context"
	"github.com/go-pg/pg/v10"
	"github.com/go-redis/cache/v8"
	"github.com/go-redis/redis/v8"
	manager2 "github.com/sakuraapp/gateway/internal/manager"
	"github.com/sakuraapp/gateway/internal/repository"
	"github.com/sakuraapp/gateway/pkg/config"
	"github.com/sakuraapp/gateway/pkg/util"
	"github.com/sakuraapp/shared/model"
	"github.com/sakuraapp/shared/resource"
)

type App interface {
	Context() context.Context
	NodeId() string
	GetConfig() *config.Config
	GetBuilder() *resource.Builder
	GetCrawler() *util.Crawler
	GetJWT() *util.JWT
	GetDB() *pg.DB
	GetRepos() *repository.Repositories
	GetRedis() *redis.Client
	GetCache() *cache.Cache
	GetHandlerMgr() *manager2.HandlerManager
	GetClientMgr() *manager2.ClientManager
	GetSessionMgr() *manager2.SessionManager
	GetRoomMgr() *manager2.RoomManager
	Dispatch(msg resource.ServerMessage) error
	DispatchLocal(msg resource.ServerMessage) error
	DispatchRoom(roomId model.RoomId, msg resource.ServerMessage) error
	DispatchRoomLocal(roomId model.RoomId, msg resource.ServerMessage) error
}