package progress

import (
	"context"
	"fmt"
	"io"
)

type plainWriter struct {
	out  io.Writer
	done chan bool
}

func (p *plainWriter) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return nil
	}
}

func (p *plainWriter) Event(e Event) {
	fmt.Println(e.ID, e.Text, e.StatusText)
}

func (p *plainWriter) Stop() {
	p.done <- true
}
