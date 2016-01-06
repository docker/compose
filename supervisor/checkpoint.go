package supervisor

type CreateCheckpointEvent struct {
	s *Supervisor
}

func (h *CreateCheckpointEvent) Handle(e *Event) error {
	/*
		i, ok := h.s.containers[e.ID]
		if !ok {
			return ErrContainerNotFound
		}
	*/
	return nil
	// return i.container.Checkpoint(*e.Checkpoint)
}

type DeleteCheckpointEvent struct {
	s *Supervisor
}

func (h *DeleteCheckpointEvent) Handle(e *Event) error {
	/*
		i, ok := h.s.containers[e.ID]
		if !ok {
			return ErrContainerNotFound
		}
	*/
	return nil
	// return i.container.DeleteCheckpoint(e.Checkpoint.Name)
}
