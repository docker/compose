package supervisor

import (
	"time"

	"github.com/docker/containerd/runtime"
)

type UpdateTask struct {
	s *Supervisor
}

func (h *UpdateTask) Handle(e *Task) error {
	i, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	container := i.container
	if e.State != "" {
		switch e.State {
		case runtime.Running:
			if err := container.Resume(); err != nil {
				return ErrUnknownContainerStatus
			}
			h.s.notifySubscribers(Event{
				ID:        e.ID,
				Type:      "resume",
				Timestamp: time.Now(),
			})
		case runtime.Paused:
			if err := container.Pause(); err != nil {
				return ErrUnknownContainerStatus
			}
			h.s.notifySubscribers(Event{
				ID:        e.ID,
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
	s *Supervisor
}

func (h *UpdateProcessTask) Handle(e *Task) error {
	i, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	processes, err := i.container.Processes()
	if err != nil {
		return err
	}
	var process runtime.Process
	for _, p := range processes {
		if p.ID() == e.Pid {
			process = p
			break
		}
	}
	if process == nil {
		return ErrProcessNotFound
	}
	if e.CloseStdin {
		if err := process.CloseStdin(); err != nil {
			return err
		}
	}
	if e.Width > 0 || e.Height > 0 {
		if err := process.Resize(e.Width, e.Height); err != nil {
			return err
		}
	}
	return nil
}
