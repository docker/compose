package supervisor

import (
	"time"

	"github.com/docker/containerd/runtime"
)

type StartEvent struct {
	s *Supervisor
}

func (h *StartEvent) Handle(e *Event) error {
	start := time.Now()
	container, err := runtime.New(h.s.stateDir, e.ID, e.BundlePath)
	if err != nil {
		return err
	}
	h.s.containers[e.ID] = &containerInfo{
		container: container,
	}
	ContainersCounter.Inc(1)
	task := &StartTask{
		Err:           e.Err,
		Container:     container,
		StartResponse: e.StartResponse,
	}
	if e.Checkpoint != nil {
		task.Checkpoint = e.Checkpoint.Name
	}
	h.s.tasks <- task
	ContainerCreateTimer.UpdateSince(start)
	return errDeferedResponse
}
