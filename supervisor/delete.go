package supervisor

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
)

type DeleteEvent struct {
	s *Supervisor
}

func (h *DeleteEvent) Handle(e *Event) error {
	if i, ok := h.s.containers[e.ID]; ok {
		start := time.Now()
		if err := h.deleteContainer(i.container); err != nil {
			logrus.WithField("error", err).Error("containerd: deleting container")
		}
		if i.copier != nil {
			if err := i.copier.Close(); err != nil {
				logrus.WithField("error", err).Error("containerd: close container copier")
			}
		}
		h.s.notifySubscribers(&Event{
			Type:   ExitEventType,
			ID:     e.ID,
			Status: e.Status,
			Pid:    e.Pid,
		})
		ContainersCounter.Dec(1)
		ContainerDeleteTimer.UpdateSince(start)
	}
	return nil
}

func (h *DeleteEvent) deleteContainer(container runtime.Container) error {
	delete(h.s.containers, container.ID())
	return container.Delete()
}
