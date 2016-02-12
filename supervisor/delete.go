package supervisor

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
)

type DeleteTask struct {
	s *Supervisor
}

func (h *DeleteTask) Handle(e *Task) error {
	if i, ok := h.s.containers[e.ID]; ok {
		start := time.Now()
		if err := h.deleteContainer(i.container); err != nil {
			logrus.WithField("error", err).Error("containerd: deleting container")
		}
		h.s.notifySubscribers(Event{
			Type:      "exit",
			Timestamp: time.Now(),
			ID:        e.ID,
			Status:    e.Status,
			Pid:       e.Pid,
		})
		ContainersCounter.Dec(1)
		ContainerDeleteTimer.UpdateSince(start)
	}
	return nil
}

func (h *DeleteTask) deleteContainer(container runtime.Container) error {
	delete(h.s.containers, container.ID())
	return container.Delete()
}
