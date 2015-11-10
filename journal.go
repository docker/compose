package containerd

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type entry struct {
	Event *Event `json:"event"`
}

func newJournal(path string) (*journal, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0755)
	if err != nil {
		return nil, err
	}
	return &journal{
		f:   f,
		enc: json.NewEncoder(f),
	}, nil
}

type journal struct {
	f   *os.File
	enc *json.Encoder
}

func (j *journal) write(e *Event) error {
	et := &entry{
		Event: e,
	}
	return j.enc.Encode(et)
}

func (j *journal) Close() error {
	return j.f.Close()
}
