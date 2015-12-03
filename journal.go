package containerd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/Sirupsen/logrus"
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
	j := &journal{
		f:   f,
		enc: json.NewEncoder(f),
		wc:  make(chan *Event, 2048),
	}
	j.wg.Add(1)
	go j.start()
	return j, nil
}

type journal struct {
	f   *os.File
	enc *json.Encoder
	wc  chan *Event
	wg  sync.WaitGroup
}

func (j *journal) start() {
	defer j.wg.Done()
	for e := range j.wc {
		et := &entry{
			Event: e,
		}
		if err := j.enc.Encode(et); err != nil {
			logrus.WithField("error", err).Error("write event to journal")
		}
	}
}

func (j *journal) write(e *Event) {
	j.wc <- e
}

func (j *journal) Close() error {
	close(j.wc)
	j.wg.Wait()
	return j.f.Close()
}
