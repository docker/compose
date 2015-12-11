package containerd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
)

type logConfig struct {
	BundlePath string
	LogSize    int64 // in bytes
	Stdin      io.WriteCloser
	Stdout     io.ReadCloser
	Stderr     io.ReadCloser
}

func newLogger(i *logConfig) (*logger, error) {
	l := &logger{
		config:   i,
		messages: make(chan *Message, DefaultBufferSize),
	}
	hout := &logHandler{
		stream:   "stdout",
		messages: l.messages,
	}
	herr := &logHandler{
		stream:   "stderr",
		messages: l.messages,
	}
	l.wg.Add(2)
	go func() {
		defer l.wg.Done()
		io.Copy(hout, i.Stdout)
	}()
	go func() {
		defer l.wg.Done()
		io.Copy(herr, i.Stderr)
	}()
	return l, l.start()
}

type Message struct {
	Stream    string    `json:"stream"`
	Timestamp time.Time `json:"timestamp"`
	Data      []byte    `json:"data"`
}

type logger struct {
	config   *logConfig
	f        *os.File
	wg       sync.WaitGroup
	messages chan *Message
}

type logHandler struct {
	stream   string
	messages chan *Message
}

func (h *logHandler) Write(b []byte) (int, error) {
	h.messages <- &Message{
		Stream:    h.stream,
		Timestamp: time.Now(),
		Data:      b,
	}
	return len(b), nil
}

func (l *logger) start() error {
	f, err := os.OpenFile(
		filepath.Join(l.config.BundlePath, "logs.json"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0655,
	)
	if err != nil {
		return err
	}
	l.f = f
	l.wg.Add(1)
	go func() {
		l.wg.Done()
		enc := json.NewEncoder(f)
		for m := range l.messages {
			if err := enc.Encode(m); err != nil {
				logrus.WithField("error", err).Error("write log message")
			}
		}
	}()
	return nil
}

func (l *logger) Close() (err error) {
	for _, c := range []io.Closer{
		l.config.Stdin,
		l.config.Stdout,
		l.config.Stderr,
	} {
		if cerr := c.Close(); err == nil {
			err = cerr
		}
	}
	close(l.messages)
	l.wg.Wait()
	if ferr := l.f.Close(); err == nil {
		err = ferr
	}
	return err
}
