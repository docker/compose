package supervisor

import (
	"sync"

	"github.com/docker/containerd/runtime"
)

// StartResponse is the response containing a started container
type StartResponse struct {
	Container runtime.Container
}

// Task executes an action returning an error chan with either nil or
// the error from executing the task
type Task interface {
	// ErrorCh returns a channel used to report and error from an async task
	ErrorCh() chan error
}

type baseTask struct {
	errCh chan error
	mu    sync.Mutex
}

func (t *baseTask) ErrorCh() chan error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.errCh == nil {
		t.errCh = make(chan error, 1)
	}
	return t.errCh
}
