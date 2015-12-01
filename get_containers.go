package containerd

type GetContainersEvent struct {
	s *Supervisor
}

func (h *GetContainersEvent) Handle(e *Event) error {
	for _, c := range h.s.containers {
		e.Containers = append(e.Containers, c)
	}
	return nil
}
