package containerd

import "github.com/Sirupsen/logrus"

type ExitEvent struct {
	s *Supervisor
}

func (h *ExitEvent) Handle(e *Event) error {
	logrus.WithFields(logrus.Fields{"pid": e.Pid, "status": e.Status}).
		Debug("containerd: process exited")
	// is it the child process of a container
	if info, ok := h.s.processes[e.Pid]; ok {
		ne := NewEvent(ExecExitEventType)
		ne.ID = info.container.ID()
		ne.Pid = e.Pid
		ne.Status = e.Status
		h.s.SendEvent(ne)
		return nil
	}

	// is it the main container's process
	container, err := h.s.getContainerForPid(e.Pid)
	if err != nil {
		if err != errNoContainerForPid {
			logrus.WithField("error", err).Error("containerd: find container for pid")
		}
		return nil
	}
	container.SetExited(e.Status)
	ne := NewEvent(DeleteEventType)
	ne.ID = container.ID()
	ne.Pid = e.Pid
	ne.Status = e.Status
	h.s.SendEvent(ne)
	return nil
}

type ExecExitEvent struct {
	s *Supervisor
}

func (h *ExecExitEvent) Handle(e *Event) error {
	// exec process: we remove this process without notifying the main event loop
	info := h.s.processes[e.Pid]
	if err := info.container.RemoveProcess(e.Pid); err != nil {
		logrus.WithField("error", err).Error("containerd: find container for pid")
	}
	if err := info.logger.Close(); err != nil {
		logrus.WithField("error", err).Error("containerd: close process IO")
	}
	delete(h.s.processes, e.Pid)
	h.s.notifySubscribers(e)
	return nil
}
