// +build !windows

package supervisor

import "github.com/docker/containerd/runtime"

// CreateCheckpointTask holds needed parameters to create a new checkpoint
type CreateCheckpointTask struct {
	baseTask
	ID            string
	CheckpointDir string
	Checkpoint    *runtime.Checkpoint
}

func (s *Supervisor) createCheckpoint(t *CreateCheckpointTask) error {
	i, ok := s.containers[t.ID]
	if !ok {
		return ErrContainerNotFound
	}
	return i.container.Checkpoint(*t.Checkpoint, t.CheckpointDir)
}

// DeleteCheckpointTask holds needed parameters to delete a checkpoint
type DeleteCheckpointTask struct {
	baseTask
	ID            string
	CheckpointDir string
	Checkpoint    *runtime.Checkpoint
}

func (s *Supervisor) deleteCheckpoint(t *DeleteCheckpointTask) error {
	i, ok := s.containers[t.ID]
	if !ok {
		return ErrContainerNotFound
	}
	return i.container.DeleteCheckpoint(t.Checkpoint.Name, t.CheckpointDir)
}
