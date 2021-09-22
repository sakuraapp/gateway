package handlers

import (
	"fmt"
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/gateway/resources"
	"github.com/sakuraapp/shared/resources/opcodes"
)

type AuthHandler struct {
	Handler
}

func (h *AuthHandler) Init() {
	m := h.app.GetHandlerMgr()

	m.Register(opcodes.AUTHENTICATE, h.HandleAuth)
}

func (h *AuthHandler) HandleAuth(data *resources.Packet, client *client.Client)  {
	fmt.Println("Hello world")

	token := data.Data["token"].(string)
	claims, err := h.app.GetJWT().Parse(token)

	if err != nil {
		fmt.Printf("Auth Failed: %v\n", err)
		client.Disconnect()

		return
	}

	userId := claims["id"]
	fmt.Printf("User Id: %v\n", userId)
}