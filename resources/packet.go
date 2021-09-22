package resources

import (
	"github.com/sakuraapp/shared/resources/opcodes"
	"gopkg.in/guregu/null.v4"
)

type Packet struct {
	Opcode opcodes.Opcode `json:"op"`
	Data map[string]interface{} `json:"d"`
	Time null.Int `json:"t"`
}