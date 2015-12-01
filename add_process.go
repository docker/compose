package containerd

type AddProcessEvent struct {
	s *Supervisor
}

func (h *AddProcessEvent) Handle(e *Event) error {
	container, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	p, err := h.s.runtime.StartProcess(container, *e.Process, e.Stdio)
	if err != nil {
		return err
	}
	if e.Pid, err = p.Pid(); err != nil {
		return err
	}
	h.s.processes[e.Pid] = container
	return nil
}
