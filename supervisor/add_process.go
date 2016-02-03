package supervisor

import (
	"time"

	"github.com/docker/containerd/runtime"
)

type AddProcessEvent struct {
	s *Supervisor
}

// TODO: add this to worker for concurrent starts???  maybe not because of races where the container
// could be stopped and removed...
func (h *AddProcessEvent) Handle(e *Event) error {
	start := time.Now()
	ci, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	process, err := ci.container.Exec(e.Pid, *e.ProcessSpec, runtime.Stdio{
		Stdin:  e.Stdin,
		Stdout: e.Stdout,
		Stderr: e.Stderr,
	})
	if err != nil {
		return err
	}
	if err := h.s.monitorProcess(process); err != nil {
		return err
	}
	ExecProcessTimer.UpdateSince(start)
	e.StartResponse <- StartResponse{}
	return nil
}
