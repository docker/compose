package supervisor

import "github.com/docker/containerd/runtime"

type GetContainersTask struct {
	baseTask
	ID         string
	Containers []runtime.Container
}

func (s *Supervisor) getContainers(t *GetContainersTask) error {

	if t.ID != "" {
		ci, ok := s.containers[t.ID]
		if !ok {
			return ErrContainerNotFound
		}
		t.Containers = append(t.Containers, ci.container)

		return nil
	}

	for _, ci := range s.containers {
		t.Containers = append(t.Containers, ci.container)
	}

	return nil
}
