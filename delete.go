package containerd

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
)

type DeleteEvent struct {
	s *Supervisor
}

func (h *DeleteEvent) Handle(e *Event) error {
	if container, ok := h.s.containers[e.ID]; ok {
		if err := h.deleteContainer(container); err != nil {
			logrus.WithField("error", err).Error("containerd: deleting container")
		} else {
			ContainersCounter.Dec(1)
			h.s.containerGroup.Done()
		}
	}
	return nil
}

func (h *DeleteEvent) deleteContainer(container runtime.Container) error {
	delete(h.s.containers, container.ID())
	return container.Delete()
}
