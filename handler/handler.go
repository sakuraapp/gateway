package handler

import (
	"github.com/sakuraapp/gateway/pkg"
	"github.com/sakuraapp/shared/resource/opcode"
)

type Handlers struct {
	app pkg.App
}

func Init(app pkg.App) {
	h := Handlers{app}
	m := app.GetHandlerMgr()

	m.Register(opcode.AUTHENTICATE, h.HandleAuth)
}