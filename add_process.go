package containerd

import "github.com/Sirupsen/logrus"

type AddProcessEvent struct {
	s *Supervisor
}

// TODO: add this to worker for concurrent starts???  maybe not because of races where the container
// could be stopped and removed...
func (h *AddProcessEvent) Handle(e *Event) error {
	container, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	p, io, err := h.s.runtime.StartProcess(container, *e.Process)
	if err != nil {
		return err
	}
	if err := h.s.log(container.Path(), io); err != nil {
		// log the error but continue with the other commands
		logrus.WithFields(logrus.Fields{
			"error": err,
			"id":    e.ID,
		}).Error("log stdio")
	}
	if e.Pid, err = p.Pid(); err != nil {
		return err
	}
	h.s.processes[e.Pid] = container
	return nil
}
