package supervisor

import "github.com/docker/containerd/runtime"

type GetContainersTask struct {
	baseTask
	ID         string
	Containers []runtime.Container
}

func (s *Supervisor) getContainers(t *GetContainersTask) error {
	if t.ID != "" {
		ci := s.containers[t.ID]
		if ci == nil {
			return ErrContainerNotFound
		}
		t.Containers = append(t.Containers, ci.container)
		return nil
	}
	for _, i := range s.containers {
		t.Containers = append(t.Containers, i.container)
	}
	return nil
}
