package supervisor

import "github.com/docker/containerd/runtime"

type UpdateEvent struct {
	s *Supervisor
}

func (h *UpdateEvent) Handle(e *Event) error {
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
		case runtime.Paused:
			if err := container.Pause(); err != nil {
				return ErrUnknownContainerStatus
			}
		default:
			return ErrUnknownContainerStatus
		}
	}
	if e.Signal != nil {
		// signal the pid1/main process of the container
		processes, err := container.Processes()
		if err != nil {
			return err
		}
		for _, p := range processes {
			if p.ID() == runtime.InitProcessID {
				return p.Signal(e.Signal)
			}
		}
		return ErrProcessNotFound
	}
	return nil
}
