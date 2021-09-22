package internal

import "github.com/sakuraapp/gateway/managers"

type App interface {
	GetJWT() *JWT
	GetHandlerMgr() *managers.HandlerManager
	GetClientMgr() *managers.ClientManager
}