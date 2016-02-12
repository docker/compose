package supervisor

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
)

type ExitTask struct {
	s *Supervisor
}

func (h *ExitTask) Handle(e *Task) error {
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
		ne := NewTask(ExecExitTaskType)
		ne.ID = proc.Container().ID()
		ne.Pid = proc.ID()
		ne.Status = status
		ne.Process = proc
		h.s.SendTask(ne)

		return nil
	}
	container := proc.Container()
	ne := NewTask(DeleteTaskType)
	ne.ID = container.ID()
	ne.Status = status
	ne.Pid = proc.ID()
	h.s.SendTask(ne)

	ExitProcessTimer.UpdateSince(start)

	return nil
}

type ExecExitTask struct {
	s *Supervisor
}

func (h *ExecExitTask) Handle(e *Task) error {
	container := e.Process.Container()
	// exec process: we remove this process without notifying the main event loop
	if err := container.RemoveProcess(e.Pid); err != nil {
		logrus.WithField("error", err).Error("containerd: find container for pid")
	}
	h.s.notifySubscribers(Event{
		Timestamp: time.Now(),
		ID:        e.ID,
		Type:      "exit",
		Pid:       e.Pid,
		Status:    e.Status,
	})
	return nil
}
