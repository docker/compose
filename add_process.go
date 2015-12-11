package containerd

import "github.com/Sirupsen/logrus"

type AddProcessEvent struct {
	s *Supervisor
}

// TODO: add this to worker for concurrent starts???  maybe not because of races where the container
// could be stopped and removed...
func (h *AddProcessEvent) Handle(e *Event) error {
	ci, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	p, io, err := h.s.runtime.StartProcess(ci.container, *e.Process)
	if err != nil {
		return err
	}
	l, err := h.s.log(ci.container.Path(), io)
	if err != nil {
		// log the error but continue with the other commands
		logrus.WithFields(logrus.Fields{
			"error": err,
			"id":    e.ID,
		}).Error("log stdio")
	}
	if e.Pid, err = p.Pid(); err != nil {
		return err
	}
	h.s.processes[e.Pid] = &containerInfo{
		container: ci.container,
		logger:    l,
	}
	return nil
}
