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
	task := &StartTask{
		Err:       e.Err,
		Container: container,
	}
	if e.Checkpoint != nil {
		task.Checkpoint = &Checkpoint{
			Name: e.Checkpoint.Name,
			Path: e.Checkpoint.Path,
		}
	}
	h.s.tasks <- task
	return errDeferedResponse
}
