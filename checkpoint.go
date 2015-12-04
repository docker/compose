package containerd

type CreateCheckpointEvent struct {
	s *Supervisor
}

func (h *CreateCheckpointEvent) Handle(e *Event) error {
	container, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	return container.Checkpoint(*e.Checkpoint)
}

type DeleteCheckpointEvent struct {
	s *Supervisor
}

func (h *DeleteCheckpointEvent) Handle(e *Event) error {
	container, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	return container.DeleteCheckpoint(e.Checkpoint.Name)
}
