package containerd

type StartEvent struct {
	s *Supervisor
}

func (h *StartEvent) Handle(e *Event) error {
	container, io, err := h.s.runtime.Create(e.ID, e.BundlePath)
	if err != nil {
		return err
	}
	h.s.containerGroup.Add(1)
	h.s.containers[e.ID] = &containerInfo{
		container: container,
	}
	ContainersCounter.Inc(1)
	task := &StartTask{
		Err:       e.Err,
		IO:        io,
		Container: container,
	}
	if e.Checkpoint != nil {
		task.Checkpoint = e.Checkpoint.Name
	}
	h.s.tasks <- task
	return errDeferedResponse
}
