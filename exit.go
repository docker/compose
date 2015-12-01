package containerd

import "github.com/Sirupsen/logrus"

type ExitEvent struct {
	s *Supervisor
}

type ExecExitEvent struct {
	s *Supervisor
}

func (h *ExitEvent) Handle(e *Event) error {
	logrus.WithFields(logrus.Fields{"pid": e.Pid, "status": e.Status}).
		Debug("containerd: process exited")
	// is it the child process of a container
	if container, ok := h.s.processes[e.Pid]; ok {
		ne := NewEvent(ExecExitEventType)
		ne.ID = container.ID()
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
	h.s.SendEvent(ne)
	return nil
}

func (h *ExecExitEvent) Handle(e *Event) error {
	// exec process: we remove this process without notifying the main event loop
	container := h.s.processes[e.Pid]
	if err := container.RemoveProcess(e.Pid); err != nil {
		logrus.WithField("error", err).Error("containerd: find container for pid")
	}
	delete(h.s.processes, e.Pid)
	h.s.NotifySubscribers(e)
	return nil
}
