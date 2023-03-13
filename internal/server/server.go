package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-pg/pg/extra/pgdebug"
	"github.com/go-pg/pg/v10"
	"github.com/go-redis/cache/v8"
	"github.com/go-redis/redis/v8"
	"github.com/lesismal/nbio/nbhttp"
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/lesismal/nbio/taskpool"
	"github.com/rs/cors"
	"github.com/sakuraapp/gateway/internal/client"
	"github.com/sakuraapp/gateway/internal/config"
	"github.com/sakuraapp/gateway/internal/handler"
	"github.com/sakuraapp/gateway/internal/manager"
	"github.com/sakuraapp/gateway/internal/repository"
	"github.com/sakuraapp/gateway/pkg/util"
	gatewaypb "github.com/sakuraapp/protobuf/gateway"
	"github.com/sakuraapp/pubsub"
	"github.com/sakuraapp/shared/pkg/crypto"
	dispatcher "github.com/sakuraapp/shared/pkg/dispatcher/gateway"
	"github.com/sakuraapp/shared/pkg/resource"
	"github.com/sakuraapp/shared/pkg/resource/opcode"
	sharedUtil "github.com/sakuraapp/shared/pkg/util"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"net/http"
	"os"
	"os/signal"
	"time"
)

type Server struct {
	gatewaypb.UnimplementedGatewayServiceServer
	config.Config
	pubsub.Dispatcher
	cors            *cors.Cors
	taskPool        *taskpool.MixedPool
	server          *nbhttp.Server
	ctx             context.Context
	ctxCancel       context.CancelFunc
	crawler         *util.Crawler
	resourceBuilder *resource.Builder
	jwt             *util.JWT
	db              *pg.DB
	rdb             *redis.Client
	repos           *repository.Repositories
	handlers        *handler.Handlers
	cache           *cache.Cache
	clientMgr       *manager.ClientManager
	sessionMgr      *manager.SessionManager
	handlerMgr      *manager.HandlerManager
	roomMgr         *manager.RoomManager
	subscriptionMgr *dispatcher.SubscriptionManager
	pubsub          *redis.PubSub
	grpc            *grpc.Server
}

func New(conf config.Config) *Server {
	c := cors.New(cors.Options{
		AllowedOrigins:   conf.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Cache-Control", "Upgrade"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	})

	dbOpts := pg.Options{
		User:     conf.DatabaseUser,
		Password: conf.DatabasePassword,
		Database: conf.DatabaseName,
	}

	db := pg.Connect(&dbOpts)
	ctx, cancel := context.WithCancel(context.Background())

	if conf.IsDev() {
		log.SetLevel(log.DebugLevel)
		db.AddQueryHook(pgdebug.DebugHook{
			// Print all queries.
			Verbose: true,
		})
	}

	if err := db.Ping(ctx); err != nil {
		log.WithError(err).Fatal("Failed to open database connection")
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     conf.RedisAddr,
		Password: conf.RedisPassword,
		DB:       conf.RedisDatabase,
	})

	myCache := cache.New(&cache.Options{
		Redis: rdb,
		// LocalCache: cache.NewTinyLFU(1000, time.Minute),
		// until server-assisted client cache is possible, don't keep a client cache (we can't invalidate it)
	})

	repos := repository.Init(db, myCache)

	jwtPublicKey, err := crypto.LoadRSAPublicKey(conf.JWTPublicPath)

	if err != nil {
		log.WithError(err).Fatal("Failed to load public key")
	}

	s3Config := &sharedUtil.S3Config{
		Bucket:         conf.S3Bucket,
		Region:         conf.S3Region,
		Endpoint:       conf.S3Endpoint,
		ForcePathStyle: conf.S3ForcePathStyle,
	}
	s3BaseUrl := sharedUtil.GetS3BaseUrl(s3Config)

	resourceBuilder := resource.NewBuilder()
	resourceBuilder.SetUserFormatter(func(user *resource.User) *resource.User {
		if !user.Avatar.IsZero() {
			user.Avatar.String = sharedUtil.ResolveS3URL(s3BaseUrl, user.Avatar.String)
		}

		return user
	})

	addr := fmt.Sprintf("0.0.0.0:%v", conf.Port)
	serverConfig := nbhttp.Config{
		Network:                 "tcp",
		Addrs:                   []string{addr},
		MaxLoad:                 1000000,
		ReleaseWebsocketPayload: true,
	}

	s := &Server{
		Config:          conf,
		cors:            c,
		ctx:             context.Background(),
		ctxCancel:       cancel,
		crawler:         util.NewCrawler(),
		resourceBuilder: resourceBuilder,
		taskPool:        util.NewTaskpool(&serverConfig),
		jwt:             &util.JWT{PublicKey: jwtPublicKey},
		db:              db,
		rdb:             rdb,
		cache:           myCache,
		repos:           repos,
		clientMgr:       manager.NewClientManager(),
		sessionMgr:      manager.NewSessionManager(),
		handlerMgr:      manager.NewHandlerManager(),
	}

	s.Dispatcher = pubsub.NewRedisDispatcher(s.ctx, s.NodeId(), s.rdb)
	s.initPubsub()

	s.subscriptionMgr = dispatcher.NewSubscriptionManager(s.pubsub)
	s.roomMgr = manager.NewRoomManager(s.subscriptionMgr)

	s.handlers = handler.Init(s)

	go s.initGrpc()

	mux := &http.ServeMux{}
	mux.HandleFunc("/", s.onConnection)

	h := c.Handler(mux)

	s.server = nbhttp.NewServer(serverConfig, h, s.taskPool.Go)

	return s
}

func (s *Server) Context() context.Context {
	return s.ctx
}

func (s *Server) NodeId() string {
	return s.Config.NodeId
}

func (s *Server) GetConfig() *config.Config {
	return &s.Config
}

func (s *Server) GetBuilder() *resource.Builder {
	return s.resourceBuilder
}

func (s *Server) GetCrawler() *util.Crawler {
	return s.crawler
}

func (s *Server) GetJWT() *util.JWT {
	return s.jwt
}

func (s *Server) GetDB() *pg.DB {
	return s.db
}

func (s *Server) GetRepos() *repository.Repositories {
	return s.repos
}

func (s *Server) GetRedis() *redis.Client {
	return s.rdb
}

func (s *Server) GetCache() *cache.Cache {
	return s.cache
}

func (s *Server) GetHandlerMgr() *manager.HandlerManager {
	return s.handlerMgr
}

func (s *Server) GetClientMgr() *manager.ClientManager {
	return s.clientMgr
}

func (s *Server) GetSessionMgr() *manager.SessionManager {
	return s.sessionMgr
}

func (s *Server) GetRoomMgr() *manager.RoomManager {
	return s.roomMgr
}

func (s *Server) GetSubscriptionMgr() *dispatcher.SubscriptionManager {
	return s.subscriptionMgr
}

func (s *Server) Start() error {
	err := s.server.Start()

	go s.clientMgr.StartTicker()
	defer s.clientMgr.StopTicker()

	if err != nil {
		return err
	}

	log.Printf("Server is listening on port %v", s.Port)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	<-interrupt

	// ctx, cancel := context.WithTimeout(context.Background(), time.Second * 5)

	// defer cancel()
	defer s.ctxCancel()

	log.Println("Shutting down...")
	err = s.server.Shutdown(s.ctx)

	if err != nil {
		panic(err)
	}

	s.grpc.GracefulStop()

	return nil
}

func (s *Server) newUpgrader() *websocket.Upgrader {
	u := websocket.NewUpgrader()
	u.CheckOrigin = s.cors.OriginAllowed

	return u
}

func (s *Server) onConnection(w http.ResponseWriter, r *http.Request) {
	u := s.newUpgrader()

	conn, err := u.Upgrade(w, r, nil)

	if err != nil {
		log.WithError(err).Error("Failed to upgrade connection")
		return
	}

	wsConn := conn.(*websocket.Conn)
	c := client.NewClient(s.ctx, wsConn, u)
	c.Session = client.NewSession(0, s.NodeId())

	s.clientMgr.Add(c)

	u.OnMessage(func(conn *websocket.Conn, messageType websocket.MessageType, data []byte) {
		c.LastActive = time.Now()
		err = conn.SetReadDeadline(time.Now().Add(manager.KeepAliveTimeout))

		if err != nil {
			log.WithError(err).Error("Failed to set read deadline")
		}

		var packet resource.Packet

		err = json.Unmarshal(data, &packet)

		if err != nil {
			log.Warnf("Received an invalid packet: %v", string(data))
			return
		}

		if packet.Opcode == opcode.Disconnect {
			return // opcode not allowed
		}

		log.Debugf("OnMessage: %+v", packet)
		s.handlerMgr.Handle(&packet, c)
	})

	u.SetPongHandler(func(conn *websocket.Conn, s string) {
		c.LastActive = time.Now()
		err = conn.SetReadDeadline(time.Now().Add(manager.KeepAliveTimeout))

		if err != nil {
			log.WithError(err).Error("Failed to set read deadline")
		}
	})

	err = wsConn.SetReadDeadline(time.Now().Add(manager.KeepAliveTimeout))

	if err != nil {
		log.WithError(err).Error("Failed to set read deadline")
	}

	wsConn.OnClose(func(conn *websocket.Conn, err error) {
		s.clientMgr.Remove(c)

		if err != nil {
			log.WithError(err).Error("Socket Closed")
		}

		session := c.Session

		if session != nil {
			s.sessionMgr.Remove(c.Session)

			disconnectPacket := resource.BuildPacket(opcode.Disconnect, nil)
			s.handlerMgr.Handle(&disconnectPacket, c)
		}
	})
}
