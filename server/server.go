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
	"github.com/rs/cors"
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/gateway/config"
	"github.com/sakuraapp/gateway/handler"
	"github.com/sakuraapp/gateway/internal"
	"github.com/sakuraapp/gateway/manager"
	"github.com/sakuraapp/gateway/repository"
	shared "github.com/sakuraapp/shared/pkg"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

type Server struct {
	config.Config
	cors *cors.Cors
	server *nbhttp.Server
	ctx context.Context
	ctxCancel context.CancelFunc
	jwt *internal.JWT
	db *pg.DB
	rdb *redis.Client
	repos *repository.Repositories
	cache *cache.Cache
	clients *manager.ClientManager
	handlers *manager.HandlerManager
	rooms *manager.RoomManager
	subscriptions *manager.SubscriptionManager
	pubsub *redis.PubSub
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
		User: os.Getenv("DB_USER"),
		Password: os.Getenv("DB_PASSWORD"),
		Database: os.Getenv("DB_DATABASE"),
	}

	db := pg.Connect(&dbOpts)
	ctx, cancel := context.WithCancel(context.Background())

	db.AddQueryHook(pgdebug.DebugHook{
		// Print all queries.
		Verbose: true,
	})

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("Error opening database connection: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: conf.RedisAddr,
		Password: conf.RedisPassword,
		DB: conf.RedisDatabase,
	})

	myCache := cache.New(&cache.Options{
		Redis: rdb,
		LocalCache: cache.NewTinyLFU(1000, time.Minute),
	})

	repos := repository.Init(db, myCache)

	jwtPublicKey := shared.LoadRSAPublicKey(conf.JWTPublicPath)

	addr := fmt.Sprintf("0.0.0.0:%v", conf.Port)
	s := &Server{
		Config:   conf,
		cors:     c,
		ctx:  	  context.Background(),
		ctxCancel: cancel,
		jwt:      &internal.JWT{PublicKey: jwtPublicKey},
		db:       db,
		rdb:      rdb,
		cache:    myCache,
		repos: 	  repos,
		clients:  manager.NewClientManager(),
		handlers: manager.NewHandlerManager(),
	}

	s.initPubsub()

	s.subscriptions = manager.NewSubscriptionManager(s.ctx, s.pubsub)
	s.rooms = manager.NewRoomManager(s.subscriptions)

	handler.Init(s)

	mux := &http.ServeMux{}
	mux.HandleFunc("/", s.onConnection)

	h := c.Handler(mux)

	s.server = nbhttp.NewServer(nbhttp.Config{
		Network: "tcp",
		Addrs: []string{addr},
		MaxLoad: 1000000,
		ReleaseWebsocketPayload: true,
	}, h, nil)

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

func (s *Server) GetJWT() *internal.JWT {
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
	return s.handlers
}

func (s *Server) GetClientMgr() *manager.ClientManager {
	return s.clients
}

func (s *Server) GetRoomMgr() *manager.RoomManager {
	return s.rooms
}

func (s *Server) Start() error {
	err := s.server.Start()

	go s.clients.StartTicker()
	defer s.clients.StopTicker()

	if err != nil {
		return err
	}

	fmt.Printf("Server is listening on port %v\n", s.Port)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	<-interrupt

	// ctx, cancel := context.WithTimeout(context.Background(), time.Second * 5)

	// defer cancel()
	defer s.ctxCancel()

	fmt.Println("Shutting down...")
	err = s.server.Shutdown(s.ctx)

	if err != nil {
		panic(err)
	}

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
		panic(err)
	}

	wsConn := conn.(*websocket.Conn)
	c := client.NewClient(s.ctx, wsConn, u)

	s.clients.Add(c)

	u.OnMessage(func(conn *websocket.Conn, messageType websocket.MessageType, data []byte) {
		c.LastActive = time.Now()
		err = conn.SetReadDeadline(time.Now().Add(manager.KeepAliveTimeout))

		if err != nil {
			panic(err)
		}

		var packet resource.Packet

		err = json.Unmarshal(data, &packet)

		if err != nil {
			fmt.Printf("Invalid Packet: %v\n", string(data))
			return
		}

		if packet.Opcode == opcode.DISCONNECT {
			return // opcode not allowed
		}

		fmt.Printf("OnMessage: %+v\n", packet)
		s.handlers.Handle(&packet, c)
	})

	u.SetPongHandler(func(conn *websocket.Conn, s string) {
		c.LastActive = time.Now()
		err = conn.SetReadDeadline(time.Now().Add(manager.KeepAliveTimeout))

		if err != nil {
			panic(err)
		}
	})

	err = wsConn.SetReadDeadline(time.Now().Add(manager.KeepAliveTimeout))

	if err != nil {
		panic(err)
	}

	wsConn.OnClose(func(conn *websocket.Conn, err error) {
		s.clients.Remove(c)

		if err != nil {
			fmt.Printf("Socket Closed: %v\n", err)
		}

		session := c.Session

		if session != nil {
			disconnectPacket := resource.BuildPacket(opcode.DISCONNECT, nil)

			s.handlers.Handle(&disconnectPacket, c)
		}
	})
}