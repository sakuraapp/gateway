package server

import (
	"github.com/sakuraapp/gateway/pkg"
	"github.com/sakuraapp/shared/resource"
)

type Dispatcher struct {
	app pkg.App
}

func NewDispatcher(app pkg.App) *Dispatcher {
	return &Dispatcher{app: app}
}

func (d *Dispatcher) DispatchLocal(msg resource.ServerMessage) {

}