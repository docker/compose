package supervisor

type SignalTask struct {
	s *Supervisor
}

func (h *SignalTask) Handle(e *Task) error {
	i, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	processes, err := i.container.Processes()
	if err != nil {
		return err
	}
	for _, p := range processes {
		if p.ID() == e.Pid {
			return p.Signal(e.Signal)
		}
	}
	return ErrProcessNotFound
}
