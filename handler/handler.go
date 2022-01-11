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
	m.Register(opcode.QueueRemove, h.HandleQueueRemove)
	m.Register(opcode.PlayerState, h.HandleSetPlayerState)
	m.Register(opcode.Seek, h.HandleSeek)
	m.Register(opcode.VideoSkip, h.HandleSkip)
	m.Register(opcode.VideoEnd, h.HandleVideoEnd)
	m.Register(opcode.KickUser, h.HandleKickUser)

	m.RegisterServer(opcode.KickUser, h.kickUser)
}