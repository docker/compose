package containerd

import (
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/rcrowley/go-metrics"
)

var (
	containerStartTimer = metrics.NewTimer()
)

// NewSupervisor returns an initialized Process supervisor.
func NewSupervisor(stateDir string, concurrency int) (*Supervisor, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, err
	}
	// register counters
	metrics.DefaultRegistry.Register("container-start-time", containerStartTimer)
	runtime, err := NewRuntime(stateDir)
	if err != nil {
		return nil, err
	}
	s := &Supervisor{
		stateDir:   stateDir,
		containers: make(map[string]Container),
		runtime:    runtime,
	}
	return s, nil
}

type Supervisor struct {
	// stateDir is the directory on the system to store container runtime state information.
	stateDir string

	containers map[string]Container

	runtime Runtime

	events chan Event
}

// Start is a non-blocking call that runs the supervisor for monitoring contianer processes and
// executing new containers.
//
// This event loop is the only thing that is allowed to modify state of containers and processes.
func (s *Supervisor) Start(events chan Event) error {
	if events == nil {
		return ErrEventChanNil
	}
	s.events = events
	go func() {
		for evt := range events {
			logrus.WithField("event", evt).Debug("containerd: processing event")
			switch e := evt.(type) {
			case *ExitEvent:
				logrus.WithFields(logrus.Fields{
					"pid":    e.Pid,
					"status": e.Status,
				}).Debug("containerd: process exited")
				container, err := s.getContainerForPid(e.Pid)
				if err != nil {
					if err != errNoContainerForPid {
						logrus.WithField("error", err).Error("containerd: find container for pid")
					}
					continue
				}
				container.SetExited(e.Status)
				delete(s.containers, container.ID())
				if err := container.Delete(); err != nil {
					logrus.WithField("error", err).Error("containerd: deleting container")
				}
			case *CreateContainerEvent:
				start := time.Now()
				container, err := s.runtime.Create(e.ID, e.BundlePath)
				if err != nil {
					e.Err <- err
					continue
				}
				s.containers[e.ID] = container
				if err := container.Start(); err != nil {
					e.Err <- err
					continue
				}
				e.Err <- nil
				containerStartTimer.UpdateSince(start)
			}
		}
	}()
	return nil
}

func (s *Supervisor) getContainerForPid(pid int) (Container, error) {
	for _, container := range s.containers {
		cpid, err := container.Pid()
		if err != nil {
			if lerr, ok := err.(libcontainer.Error); ok {
				if lerr.Code() == libcontainer.ProcessNotExecuted {
					continue
				}
			}
			logrus.WithField("error", err).Error("containerd: get container pid")
		}
		if pid == cpid {
			return container, nil
		}
	}
	return nil, errNoContainerForPid
}

func (s *Supervisor) SendEvent(evt Event) {
	s.events <- evt
}

// Stop initiates a shutdown of the supervisor killing all processes under supervision.
func (s *Supervisor) Stop() {

}
