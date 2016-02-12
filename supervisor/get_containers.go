package supervisor

type GetContainersTask struct {
	s *Supervisor
}

func (h *GetContainersTask) Handle(e *Task) error {
	if e.ID != "" {
		ci := h.s.containers[e.ID]
		if ci == nil {
			return ErrContainerNotFound
		}
		e.Containers = append(e.Containers, ci.container)
		return nil
	}
	for _, i := range h.s.containers {
		e.Containers = append(e.Containers, i.container)
	}
	return nil
}
