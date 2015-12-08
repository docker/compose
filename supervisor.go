package containerd

import (
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
	"github.com/opencontainers/runc/libcontainer"
)

// NewSupervisor returns an initialized Process supervisor.
func NewSupervisor(id, stateDir string, tasks chan *StartTask) (*Supervisor, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, err
	}
	// register counters
	r, err := newRuntime(filepath.Join(stateDir, id))
	if err != nil {
		return nil, err
	}
	machine, err := CollectMachineInformation(id)
	if err != nil {
		return nil, err
	}
	s := &Supervisor{
		stateDir:   stateDir,
		containers: make(map[string]runtime.Container),
		processes:  make(map[int]runtime.Container),
		runtime:    r,
		tasks:      tasks,
		events:     make(chan *Event, 2048),
		machine:    machine,
	}
	// register default event handlers
	s.handlers = map[EventType]Handler{
		ExecExitEventType:         &ExecExitEvent{s},
		ExitEventType:             &ExitEvent{s},
		StartContainerEventType:   &StartEvent{s},
		DeleteEventType:           &DeleteEvent{s},
		GetContainerEventType:     &GetContainersEvent{s},
		SignalEventType:           &SignalEvent{s},
		AddProcessEventType:       &AddProcessEvent{s},
		UpdateContainerEventType:  &UpdateEvent{s},
		CreateCheckpointEventType: &CreateCheckpointEvent{s},
		DeleteCheckpointEventType: &DeleteCheckpointEvent{s},
	}
	// start the container workers for concurrent container starts
	return s, nil
}

type Supervisor struct {
	// stateDir is the directory on the system to store container runtime state information.
	stateDir       string
	containers     map[string]runtime.Container
	processes      map[int]runtime.Container
	handlers       map[EventType]Handler
	runtime        runtime.Runtime
	events         chan *Event
	tasks          chan *StartTask
	subscribers    map[subscriber]bool
	machine        Machine
	containerGroup sync.WaitGroup
}

type subscriber chan *Event

func (s *Supervisor) Stop(sig chan os.Signal) {
	// Close the tasks channel so that no new containers get started
	close(s.tasks)
	// send a SIGTERM to all containers
	for id, c := range s.containers {
		logrus.WithField("id", id).Debug("sending TERM to container processes")
		procs, err := c.Processes()
		if err != nil {
			logrus.WithField("id", id).Warn("get container processes")
			continue
		}
		if len(procs) == 0 {
			continue
		}
		mainProc := procs[0]
		if err := mainProc.Signal(syscall.SIGTERM); err != nil {
			pid, _ := mainProc.Pid()
			logrus.WithFields(logrus.Fields{
				"id":    id,
				"pid":   pid,
				"error": err,
			}).Error("send SIGTERM to process")
		}
	}
	go func() {
		logrus.Debug("waiting for containers to exit")
		s.containerGroup.Wait()
		logrus.Debug("all containers exited")
		// stop receiving signals and close the channel
		signal.Stop(sig)
		close(sig)
	}()
}

// Close closes any open files in the supervisor but expects that Stop has been
// callsed so that no more containers are started.
func (s *Supervisor) Close() error {
	return nil
}

func (s *Supervisor) Events() subscriber {
	return subscriber(make(chan *Event))
}

func (s *Supervisor) Unsubscribe(sub subscriber) {
	delete(s.subscribers, sub)
}

func (s *Supervisor) NotifySubscribers(e *Event) {
	for sub := range s.subscribers {
		sub <- e
	}
}

// Start is a non-blocking call that runs the supervisor for monitoring contianer processes and
// executing new containers.
//
// This event loop is the only thing that is allowed to modify state of containers and processes.
func (s *Supervisor) Start() error {
	go func() {
		// allocate an entire thread to this goroutine for the main event loop
		// so that nothing else is scheduled over the top of it.
		goruntime.LockOSThread()
		for e := range s.events {
			EventsCounter.Inc(1)
			h, ok := s.handlers[e.Type]
			if !ok {
				e.Err <- ErrUnknownEvent
				continue
			}
			if err := h.Handle(e); err != nil {
				if err != errDeferedResponse {
					e.Err <- err
					close(e.Err)
				}
				continue
			}
			close(e.Err)
		}
	}()
	logrus.WithFields(logrus.Fields{
		"runtime":  s.runtime.Type(),
		"stateDir": s.stateDir,
	}).Debug("Supervisor started")
	return nil
}

// Machine returns the machine information for which the
// supervisor is executing on.
func (s *Supervisor) Machine() Machine {
	return s.machine
}

func (s *Supervisor) getContainerForPid(pid int) (runtime.Container, error) {
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

func (s *Supervisor) SendEvent(evt *Event) {
	s.events <- evt
}
