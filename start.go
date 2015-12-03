package containerd

type StartEvent struct {
	s *Supervisor
}

func (h *StartEvent) Handle(e *Event) error {
	container, err := h.s.runtime.Create(e.ID, e.BundlePath, e.Stdio)
	if err != nil {
		return err
	}
	h.s.containers[e.ID] = container
	ContainersCounter.Inc(1)
	h.s.tasks <- &StartTask{
		Err:       e.Err,
		Container: container,
	}
	return errDeferedResponse
}
