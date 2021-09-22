package server

import (
	"encoding/json"
	"fmt"
	"github.com/lesismal/nbio/nbhttp"
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/rs/cors"
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/gateway/handlers"
	"github.com/sakuraapp/gateway/internal"
	"github.com/sakuraapp/gateway/managers"
	"github.com/sakuraapp/gateway/resources"
	"github.com/sakuraapp/shared"
	"net/http"
	"time"
)

type Config struct {
	Port string
	AllowedOrigins []string
	JWTPublicPath string
}

type Server struct {
	Config
	cors *cors.Cors
	server *nbhttp.Server
	jwt *internal.JWT
	clients *managers.ClientManager
	handlers *managers.HandlerManager
}

func New(conf Config) *Server {
	c := cors.New(cors.Options{
		AllowedOrigins:   conf.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Cache-Control", "Upgrade"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	})

	jwtPublicKey := shared.LoadRSAPublicKey(conf.JWTPublicPath)

	addrs := []string{fmt.Sprintf("0.0.0.0:%v", conf.Port)}
	s := &Server{
		Config: conf,
		cors: c,
		jwt: &internal.JWT{PublicKey: jwtPublicKey},
		clients: managers.NewClientManager(),
		handlers: managers.NewHandlerManager(),
	}

	handlers.Init(s)

	mux := &http.ServeMux{}
	mux.HandleFunc("/", s.onConnection)

	handler := c.Handler(mux)

	s.server = nbhttp.NewServer(nbhttp.Config{
		Network: "tcp",
		Addrs: addrs,
		MaxLoad: 1000000,
		ReleaseWebsocketPayload: true,
	}, handler, nil)

	return s
}

func (s *Server) GetJWT() *internal.JWT {
	return s.jwt
}

func (s *Server) GetHandlerMgr() *managers.HandlerManager {
	return s.handlers
}

func (s *Server) GetClientMgr() *managers.ClientManager {
	return s.clients
}

func (s *Server) Start() error {
	err := s.server.Start()

	if err != nil {
		return err
	}

	fmt.Printf("Server is listening on port %v\n", s.Port)

	defer s.server.Stop()

	// block
	ticker := time.NewTicker(time.Second)

	for i := 1; true; i++ {
		<-ticker.C
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
	client := client.NewClient(wsConn, u)

	s.clients.Add(client)

	u.OnMessage(func(c *websocket.Conn, messageType websocket.MessageType, data []byte) {
		var packet resources.Packet

		err := json.Unmarshal(data, &packet)

		if err != nil {
			fmt.Printf("Invalid Packet: %v\n", string(data))
			return
		}

		fmt.Printf("OnMessage: %+v\n", packet)
		s.handlers.Handle(&packet, client)
	})

	wsConn.OnClose(func(conn *websocket.Conn, err error) {
		s.clients.Remove(client)
	})
}