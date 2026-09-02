package relay

import "encoding/json"

// Message types.
const (
	TypeRequest  = "request"
	TypeResponse = "response"
	TypePing     = "ping"
	TypePong     = "pong"
	TypeError    = "error"
)

// Message is the relay wire format.
type Message struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Payload []byte `json:"payload,omitempty"`
	Error   string `json:"error,omitempty"`
}

func Encode(m Message) ([]byte, error) { return json.Marshal(m) }
func Decode(data []byte) (Message, error) {
	var m Message
	return m, json.Unmarshal(data, &m)
}
