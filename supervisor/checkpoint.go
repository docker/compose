// +build !windows

package supervisor

import "github.com/docker/containerd/runtime"

type CreateCheckpointTask struct {
	baseTask
	ID         string
	Checkpoint *runtime.Checkpoint
}

func (s *Supervisor) createCheckpoint(t *CreateCheckpointTask) error {
	i, ok := s.containers[t.ID]
	if !ok {
		return ErrContainerNotFound
	}
	return i.container.Checkpoint(*t.Checkpoint)
}

type DeleteCheckpointTask struct {
	baseTask
	ID         string
	Checkpoint *runtime.Checkpoint
}

func (s *Supervisor) deleteCheckpoint(t *DeleteCheckpointTask) error {
	i, ok := s.containers[t.ID]
	if !ok {
		return ErrContainerNotFound
	}
	return i.container.DeleteCheckpoint(t.Checkpoint.Name)
}
