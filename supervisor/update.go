package supervisor

import (
	"time"

	"github.com/docker/containerd/runtime"
)

// UpdateTask holds needed parameters to update a container resource constraints
type UpdateTask struct {
	baseTask
	ID        string
	State     runtime.State
	Resources *runtime.Resource
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
				return err
			}
			s.notifySubscribers(Event{
				ID:        t.ID,
				Type:      StateResume,
				Timestamp: time.Now(),
			})
		case runtime.Paused:
			if err := container.Pause(); err != nil {
				return err
			}
			s.notifySubscribers(Event{
				ID:        t.ID,
				Type:      StatePause,
				Timestamp: time.Now(),
			})
		default:
			return ErrUnknownContainerStatus
		}
		return nil
	}
	if t.Resources != nil {
		return container.UpdateResources(t.Resources)
	}
	return nil
}

// UpdateProcessTask holds needed parameters to update a container
// process terminal size or close its stdin
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
