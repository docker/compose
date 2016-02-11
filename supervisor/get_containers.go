package supervisor

type GetContainersEvent struct {
	s *Supervisor
}

func (h *GetContainersEvent) Handle(e *Event) error {
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
