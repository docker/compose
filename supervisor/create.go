package supervisor

import (
	"time"

	"github.com/docker/containerd/runtime"
)

type StartTask struct {
	s *Supervisor
}

func (h *StartTask) Handle(e *Task) error {
	start := time.Now()
	container, err := runtime.New(h.s.stateDir, e.ID, e.BundlePath, e.Labels)
	if err != nil {
		return err
	}
	h.s.containers[e.ID] = &containerInfo{
		container: container,
	}
	ContainersCounter.Inc(1)
	task := &startTask{
		Err:           e.Err,
		Container:     container,
		StartResponse: e.StartResponse,
		Stdin:         e.Stdin,
		Stdout:        e.Stdout,
		Stderr:        e.Stderr,
	}
	if e.Checkpoint != nil {
		task.Checkpoint = e.Checkpoint.Name
	}
	h.s.tasks <- task
	ContainerCreateTimer.UpdateSince(start)
	return errDeferedResponse
}
