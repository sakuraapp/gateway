package manager

import (
	"github.com/sakuraapp/gateway/internal/client"
	"github.com/sakuraapp/gateway/internal/gateway"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
)

// Normal handlers handle client messages
// Server handles handle server messages, i.e. messages from other servers
// todo: rework this to use generics once they're out in stable

type HandlerFunc func(packet *resource.Packet, client *client.Client) gateway.Error
type HandlerList []HandlerFunc
type HandlerMap map[opcode.Opcode]HandlerList

type ServerHandlerFunc func(packet *resource.ServerMessage)
type ServerHandlerList []ServerHandlerFunc
type ServerHandlerMap map[opcode.Opcode]ServerHandlerList

type HandlerManager struct {
	handlers       HandlerMap
	serverHandlers ServerHandlerMap
}

func NewHandlerManager() *HandlerManager {
	return &HandlerManager{
		handlers:       HandlerMap{},
		serverHandlers: ServerHandlerMap{},
	}
}

func (h *HandlerManager) Register(op opcode.Opcode, fn HandlerFunc)  {
	if h.handlers[op] == nil {
		h.handlers[op] = HandlerList{fn}
	} else {
		h.handlers[op] = append(h.handlers[op], fn)
	}
}

func (h *HandlerManager) Handle(packet *resource.Packet, client *client.Client) {
	list := h.handlers[packet.Opcode]

	if list != nil {
		var gErr gateway.Error
		var err error

		for _, handler := range list {
			gErr = handler(packet, client)

			if gErr != nil {
				err = gErr.Handle(client)

				if err != nil {
					panic(err)
				}
			}
		}
	}
}

func (h *HandlerManager) RegisterServer(op opcode.Opcode, fn ServerHandlerFunc)  {
	if h.serverHandlers[op] == nil {
		h.serverHandlers[op] = ServerHandlerList{fn}
	} else {
		h.serverHandlers[op] = append(h.serverHandlers[op], fn)
	}
}

func (h *HandlerManager) HandleServer(msg *resource.ServerMessage) {
	list := h.serverHandlers[msg.Data.Opcode]

	if list != nil {
		for _, handler := range list {
			handler(msg)
		}
	}
}