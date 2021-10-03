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

	m.Register(opcode.AUTHENTICATE, h.HandleAuth)
}