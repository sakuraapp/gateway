package client

import (
	"context"
	"encoding/json"
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/sakuraapp/shared/pkg/dispatcher/gateway"
	dispatcher "github.com/sakuraapp/shared/pkg/dispatcher/gateway"
	"github.com/sakuraapp/shared/pkg/model"
	"github.com/sakuraapp/shared/pkg/resource"
	"github.com/sakuraapp/shared/pkg/resource/opcode"
	log "github.com/sirupsen/logrus"
	"time"
)

type Client struct {
	Session    *Session
	LastActive time.Time
	ctx        context.Context
	ctxCancel  context.CancelFunc
	conn       *websocket.Conn
	upgrader   *websocket.Upgrader
}

func (c *Client) Context() context.Context {
	return c.ctx
}

func (c *Client) Conn() *websocket.Conn {
	return c.conn
}

func (c *Client) Write(packet resource.Packet) error {
	b, err := json.Marshal(packet)

	if err != nil {
		return err
	}

	err = c.conn.WriteMessage(websocket.TextMessage, b)

	if err != nil {
		return err
	}

	log.Debugf("OnWrite: %+v\n", packet)

	return nil
}

func (c *Client) Send(op opcode.Opcode, data interface{}) error {
	packet := resource.BuildPacket(op, data)

	return c.Write(packet)
}

func (c *Client) Dispatch(msg *dispatcher.Message) {
	if msg.Filters != nil {
		for filter, value := range msg.Filters {
			switch filter {
			case gateway.MessageFilterRoom:
				roomId := value.(model.RoomId)

				if c.Session.RoomId != roomId {
					return
				}
			}
		}
	}

	err := c.Write(msg.Payload)

	if err != nil {
		log.
			WithField("session_id", c.Session.Id).
			WithError(err).
			Error("Failed to write dispatch a message to a client")
	}
}

func (c *Client) Disconnect() {
	c.ctxCancel()
	defer c.conn.Close()
}

func NewClient(ctx context.Context, conn *websocket.Conn, upgrader *websocket.Upgrader) *Client {
	ctx, cancel := context.WithCancel(ctx)
	c := &Client{
		ctx:       ctx,
		ctxCancel: cancel,
		conn:      conn,
		upgrader:  upgrader,
	}

	return c
}
