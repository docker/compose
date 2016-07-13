package supervisor

import "github.com/docker/containerd/runtime"

// GetContainersTask holds needed parameters to retrieve a list of
// containers
type GetContainersTask struct {
	baseTask
	ID       string
	GetState func(c runtime.Container) (interface{}, error)

	Containers []runtime.Container
	States     []interface{}
}

func (s *Supervisor) getContainers(t *GetContainersTask) error {

	if t.ID != "" {
		ci, ok := s.containers[t.ID]
		if !ok {
			return ErrContainerNotFound
		}
		t.Containers = append(t.Containers, ci.container)
		if t.GetState != nil {
			st, err := t.GetState(ci.container)
			if err != nil {
				return err
			}
			t.States = append(t.States, st)
		}

		return nil
	}

	for _, ci := range s.containers {
		t.Containers = append(t.Containers, ci.container)
		if t.GetState != nil {
			st, err := t.GetState(ci.container)
			if err != nil {
				return err
			}
			t.States = append(t.States, st)
		}
	}

	return nil
}
