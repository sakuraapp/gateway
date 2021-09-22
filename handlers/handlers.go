package handlers

import (
	"github.com/sakuraapp/gateway/internal"
)

type Handler struct {
	app internal.App
}

func Init(app internal.App) {
	auth := AuthHandler{Handler{app}}
	auth.Init()
}