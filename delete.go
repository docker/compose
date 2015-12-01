package containerd

import "github.com/Sirupsen/logrus"

type DeleteEvent struct {
	s *Supervisor
}

func (h *DeleteEvent) Handle(e *Event) error {
	if container, ok := h.s.containers[e.ID]; ok {
		if err := h.deleteContainer(container); err != nil {
			logrus.WithField("error", err).Error("containerd: deleting container")
		}
		ContainersCounter.Dec(1)
	}
	return nil
}

func (h *DeleteEvent) deleteContainer(container Container) error {
	delete(h.s.containers, container.ID())
	return container.Delete()
}
