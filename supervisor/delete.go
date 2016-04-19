package supervisor

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
)

type DeleteTask struct {
	baseTask
	ID      string
	Status  int
	PID     string
	NoEvent bool
}

func (s *Supervisor) delete(t *DeleteTask) error {
	if i, ok := s.containers[t.ID]; ok {
		start := time.Now()
		if err := s.deleteContainer(i.container); err != nil {
			logrus.WithField("error", err).Error("containerd: deleting container")
		}
		if !t.NoEvent {
			s.notifySubscribers(Event{
				Type:      StateExit,
				Timestamp: time.Now(),
				ID:        t.ID,
				Status:    t.Status,
				PID:       t.PID,
			})
		}
		ContainersCounter.Dec(1)
		ContainerDeleteTimer.UpdateSince(start)
	}
	return nil
}

func (s *Supervisor) deleteContainer(container runtime.Container) error {
	delete(s.containers, container.ID())
	return container.Delete()
}
