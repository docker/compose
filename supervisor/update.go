package supervisor

import (
	"time"

	"github.com/docker/containerd/runtime"
)

type UpdateTask struct {
	baseTask
	ID    string
	State runtime.State
}

func (s *Supervisor) updateContainer(t *UpdateTask) error {
	i, ok := s.containers[t.ID]
	if !ok {
		return ErrContainerNotFound
	}
	container := i.container
	if t.State != "" {
		switch t.State {
		case runtime.Running:
			if err := container.Resume(); err != nil {
				return ErrUnknownContainerStatus
			}
			s.notifySubscribers(Event{
				ID:        t.ID,
				Type:      "resume",
				Timestamp: time.Now(),
			})
		case runtime.Paused:
			if err := container.Pause(); err != nil {
				return ErrUnknownContainerStatus
			}
			s.notifySubscribers(Event{
				ID:        t.ID,
				Type:      "pause",
				Timestamp: time.Now(),
			})
		default:
			return ErrUnknownContainerStatus
		}
	}
	return nil
}

type UpdateProcessTask struct {
	baseTask
	ID         string
	PID        string
	CloseStdin bool
	Width      int
	Height     int
}

func (s *Supervisor) updateProcess(t *UpdateProcessTask) error {
	i, ok := s.containers[t.ID]
	if !ok {
		return ErrContainerNotFound
	}
	processes, err := i.container.Processes()
	if err != nil {
		return err
	}
	var process runtime.Process
	for _, p := range processes {
		if p.ID() == t.PID {
			process = p
			break
		}
	}
	if process == nil {
		return ErrProcessNotFound
	}
	if t.CloseStdin {
		if err := process.CloseStdin(); err != nil {
			return err
		}
	}
	if t.Width > 0 || t.Height > 0 {
		if err := process.Resize(t.Width, t.Height); err != nil {
			return err
		}
	}
	return nil
}
