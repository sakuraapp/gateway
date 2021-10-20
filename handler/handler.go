package handler

import (
	"github.com/sakuraapp/gateway/internal"
	"github.com/sakuraapp/shared/resource/opcode"
)

type Handlers struct {
	app internal.App
}

func Init(app internal.App) {
	h := Handlers{app}
	m := app.GetHandlerMgr()

	m.Register(opcode.Authenticate, h.HandleAuth)
	m.Register(opcode.Disconnect, h.HandleDisconnect)
	m.Register(opcode.JoinRoom, h.HandleJoinRoom)
	m.Register(opcode.LeaveRoom, h.HandleLeaveRoom)
	m.Register(opcode.QueueAdd, h.HandleQueueAdd)
}