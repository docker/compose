package containerd

type UpdateEvent struct {
	s *Supervisor
}

func (h *UpdateEvent) Handle(e *Event) error {
	container, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	if e.State.Status != "" {
		switch e.State.Status {
		case Running:
			if err := container.Resume(); err != nil {
				return ErrUnknownContainerStatus
			}
		case Paused:
			if err := container.Pause(); err != nil {
				return ErrUnknownContainerStatus
			}
		default:
			return ErrUnknownContainerStatus
		}
	}
	return nil
}
