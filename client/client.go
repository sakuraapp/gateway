package client

import (
	"context"
	"encoding/json"
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
	"gopkg.in/guregu/null.v4"
	"time"
)

type Client struct {
	Session *Session
	LastActive time.Time
	ctx context.Context
	ctxCancel context.CancelFunc
	conn *websocket.Conn
	upgrader *websocket.Upgrader
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

	return nil
}

func (c *Client) Send(op opcode.Opcode, data interface{}) error {
	t := time.Now().UnixNano() / 1000000

	return c.Write(resource.Packet{
		Opcode: op,
		Data: data,
		Time: null.IntFrom(t),
	})
}

func (c *Client) Disconnect() {
	c.ctxCancel()
	err := c.conn.Close()

	if err != nil {
		panic(err)
	}
}

func NewClient(ctx context.Context, conn *websocket.Conn, upgrader *websocket.Upgrader) *Client {
	ctx, cancel := context.WithCancel(ctx)
	c := &Client{
		ctx: ctx,
		ctxCancel: cancel,
		conn: conn,
		upgrader: upgrader,
	}

	return c
}