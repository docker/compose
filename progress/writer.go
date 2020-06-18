package progress

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/containerd/console"
	"github.com/moby/term"
	"golang.org/x/sync/errgroup"
)

// EventStatus indicates the status of an action
type EventStatus int

const (
	// Working means that the current task is working
	Working EventStatus = iota
	// Done means that the current task is done
	Done
	// Error means that the current task has errored
	Error
)

// Event reprensents a progress event
type Event struct {
	ID         string
	Text       string
	Status     EventStatus
	StatusText string
	Done       bool

	startTime time.Time
	endTime   time.Time
	spinner   *spinner
}

func (e *Event) stop() {
	e.endTime = time.Now()
	e.spinner.Stop()
}

// Writer can write multiple progress events
type Writer interface {
	Start(context.Context) error
	Stop()
	Event(Event)
}

type writerKey struct{}

// WithContextWriter adds the writer to the context
func WithContextWriter(ctx context.Context, writer Writer) context.Context {
	return context.WithValue(ctx, writerKey{}, writer)
}

// ContextWriter returns the writer from the context
func ContextWriter(ctx context.Context) Writer {
	s, _ := ctx.Value(writerKey{}).(Writer)
	return s
}

type progressFunc func(context.Context) error

// Run will run a writer and the progress function
// in parallel
func Run(ctx context.Context, pf progressFunc) error {
	eg, _ := errgroup.WithContext(ctx)
	w, err := NewWriter(os.Stderr)
	if err != nil {
		return err
	}
	eg.Go(func() error {
		return w.Start(context.Background())
	})

	ctx = WithContextWriter(ctx, w)

	eg.Go(func() error {
		defer w.Stop()
		return pf(ctx)
	})

	return eg.Wait()
}

// NewWriter returns a new multi-progress writer
func NewWriter(out console.File) (Writer, error) {
	_, isTerminal := term.GetFdInfo(out)

	if isTerminal {
		con, err := console.ConsoleFromFile(out)
		if err != nil {
			return nil, err
		}

		return &ttyWriter{
			out:      con,
			eventIDs: []string{},
			events:   map[string]Event{},
			repeated: false,
			done:     make(chan bool),
			mtx:      &sync.RWMutex{},
		}, nil
	}

	return &plainWriter{
		out:  out,
		done: make(chan bool),
	}, nil
}
