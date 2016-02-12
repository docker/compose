package supervisor

import (
	"time"

	"github.com/docker/containerd/runtime"
)

type AddProcessTask struct {
	s *Supervisor
}

// TODO: add this to worker for concurrent starts???  maybe not because of races where the container
// could be stopped and removed...
func (h *AddProcessTask) Handle(e *Task) error {
	start := time.Now()
	ci, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	process, err := ci.container.Exec(e.Pid, *e.ProcessSpec, runtime.NewStdio(e.Stdin, e.Stdout, e.Stderr))
	if err != nil {
		return err
	}
	if err := h.s.monitorProcess(process); err != nil {
		return err
	}
	ExecProcessTimer.UpdateSince(start)
	e.StartResponse <- StartResponse{}
	h.s.notifySubscribers(Event{
		Timestamp: time.Now(),
		Type:      "start-process",
		Pid:       e.Pid,
		ID:        e.ID,
	})
	return nil
}
