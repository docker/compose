package containerd

type SignalEvent struct {
	s *Supervisor
}

func (h *SignalEvent) Handle(e *Event) error {
	container, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	processes, err := container.Processes()
	if err != nil {
		return err
	}
	for _, p := range processes {
		if pid, err := p.Pid(); err == nil && pid == e.Pid {
			return p.Signal(e.Signal)
		}
	}
	return ErrProcessNotFound
}
