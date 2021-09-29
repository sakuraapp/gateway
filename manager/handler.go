package manager

import (
	"github.com/sakuraapp/gateway/client"
	"github.com/sakuraapp/shared/resource"
	"github.com/sakuraapp/shared/resource/opcode"
)

type HandlerFunc func(packet *resource.Packet, client *client.Client)
type HandlerList []HandlerFunc
type HandlerMap map[opcode.Opcode]HandlerList

type HandlerManager struct {
	handlers HandlerMap
}

func NewHandlerManager() *HandlerManager {
	return &HandlerManager{
		handlers: HandlerMap{},
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
		for _, handler := range list {
			handler(packet, client)
		}
	}
}