package supervisor

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
)

type ExitEvent struct {
	s *Supervisor
}

func (h *ExitEvent) Handle(e *Event) error {
	start := time.Now()
	proc := e.Process
	status, err := proc.ExitStatus()
	if err != nil {
		logrus.WithField("error", err).Error("containerd: get exit status")
	}
	logrus.WithFields(logrus.Fields{"pid": proc.ID(), "status": status}).Debug("containerd: process exited")

	// if the process is the the init process of the container then
	// fire a separate event for this process
	if proc.ID() != runtime.InitProcessID {
		ne := NewEvent(ExecExitEventType)
		ne.ID = proc.Container().ID()
		ne.Pid = proc.ID()
		ne.Status = status
		ne.Process = proc
		h.s.SendEvent(ne)

		return nil
	}
	container := proc.Container()
	ne := NewEvent(DeleteEventType)
	ne.ID = container.ID()
	ne.Status = status
	ne.Pid = proc.ID()
	h.s.SendEvent(ne)

	// remove stats collection for container
	stopCollect := NewEvent(StopStatsEventType)
	stopCollect.ID = container.ID()
	h.s.SendEvent(stopCollect)
	ExitProcessTimer.UpdateSince(start)

	return nil
}

type ExecExitEvent struct {
	s *Supervisor
}

func (h *ExecExitEvent) Handle(e *Event) error {
	container := e.Process.Container()
	// exec process: we remove this process without notifying the main event loop
	if err := container.RemoveProcess(e.Pid); err != nil {
		logrus.WithField("error", err).Error("containerd: find container for pid")
	}
	h.s.notifySubscribers(e)
	return nil
}
