package events

type Event interface{}

type Envelope struct {
	Tx    int64 `json:",omitempty"`
	Topic string
	Event interface{}
}
