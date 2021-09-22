package managers

import (
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/gateway/resources"
	"github.com/sakuraapp/shared/resources/opcodes"
)

type HandlerFunc func(packet *resources.Packet, client *client.Client)
type HandlerList []HandlerFunc
type HandlerMap map[opcodes.Opcode]HandlerList

type HandlerManager struct {
	handlers HandlerMap
}

func NewHandlerManager() *HandlerManager {
	return &HandlerManager{
		handlers: HandlerMap{},
	}
}

func (h *HandlerManager) Register(op opcodes.Opcode, fn HandlerFunc)  {
	if h.handlers[op] == nil {
		h.handlers[op] = HandlerList{fn}
	} else {
		h.handlers[op] = append(h.handlers[op], fn)
	}
}

func (h *HandlerManager) Handle(packet *resources.Packet, client *client.Client) {
	list := h.handlers[packet.Opcode]

	if list != nil {
		for _, handler := range list {
			handler(packet, client)
		}
	}
}