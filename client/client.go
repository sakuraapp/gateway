package client

import "github.com/lesismal/nbio/nbhttp/websocket"

type Client struct {
	conn *websocket.Conn
	upgrader *websocket.Upgrader
}

func (c *Client) Disconnect() {
	err := c.conn.Close()

	if err != nil {
		panic(err)
	}
}

func NewClient(conn *websocket.Conn, upgrader *websocket.Upgrader) *Client {
	return &Client{
		conn: conn,
		upgrader: upgrader,
	}
}