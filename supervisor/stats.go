package supervisor

import (
	"time"

	"github.com/docker/containerd/runtime"
)

type StatsTask struct {
	baseTask
	ID   string
	Stat chan *runtime.Stat
	Err  chan error
}

func (s *Supervisor) stats(t *StatsTask) error {
	start := time.Now()
	i, ok := s.containers[t.ID]
	if !ok {
		return ErrContainerNotFound
	}
	// TODO: use workers for this
	go func() {
		s, err := i.container.Stats()
		if err != nil {
			t.Err <- err
			return
		}
		t.Err <- nil
		t.Stat <- s
		ContainerStatsTimer.UpdateSince(start)
	}()
	return nil
}
