package supervisor

type CreateCheckpointTask struct {
	s *Supervisor
}

func (h *CreateCheckpointTask) Handle(e *Task) error {
	i, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	return i.container.Checkpoint(*e.Checkpoint)
}

type DeleteCheckpointTask struct {
	s *Supervisor
}

func (h *DeleteCheckpointTask) Handle(e *Task) error {
	i, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	return i.container.DeleteCheckpoint(e.Checkpoint.Name)
}
