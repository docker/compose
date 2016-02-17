package supervisor

import (
	"time"

	"github.com/docker/containerd/runtime"
)

type StartTask struct {
	baseTask
	ID            string
	BundlePath    string
	Stdout        string
	Stderr        string
	Stdin         string
	StartResponse chan StartResponse
	Checkpoint    *runtime.Checkpoint
	Labels        []string
}

func (s *Supervisor) start(t *StartTask) error {
	start := time.Now()
	container, err := runtime.New(s.stateDir, t.ID, t.BundlePath, t.Labels)
	if err != nil {
		return err
	}
	s.containers[t.ID] = &containerInfo{
		container: container,
	}
	ContainersCounter.Inc(1)
	task := &startTask{
		Err:           t.ErrorCh(),
		Container:     container,
		StartResponse: t.StartResponse,
		Stdin:         t.Stdin,
		Stdout:        t.Stdout,
		Stderr:        t.Stderr,
	}
	if t.Checkpoint != nil {
		task.Checkpoint = t.Checkpoint.Name
	}
	s.startTasks <- task
	ContainerCreateTimer.UpdateSince(start)
	return errDeferedResponse
}
