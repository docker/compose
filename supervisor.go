package containerd

import (
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
	"github.com/opencontainers/runc/libcontainer"
)

const statsInterval = 1 * time.Second

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
		stateDir:       stateDir,
		containers:     make(map[string]*containerInfo),
		processes:      make(map[int]*containerInfo),
		runtime:        r,
		tasks:          tasks,
		events:         make(chan *Event, DefaultBufferSize),
		machine:        machine,
		subscribers:    make(map[chan *Event]struct{}),
		statsCollector: newStatsCollector(statsInterval),
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
		StatsEventType:            &StatsEvent{s},
		UnsubscribeStatsEventType: &UnsubscribeStatsEvent{s},
	}
	// start the container workers for concurrent container starts
	return s, nil
}

type containerInfo struct {
	container runtime.Container
	copier    *copier
}

type Supervisor struct {
	// stateDir is the directory on the system to store container runtime state information.
	stateDir   string
	containers map[string]*containerInfo
	processes  map[int]*containerInfo
	handlers   map[EventType]Handler
	runtime    runtime.Runtime
	events     chan *Event
	tasks      chan *StartTask
	// we need a lock around the subscribers map only because additions and deletions from
	// the map are via the API so we cannot really control the concurrency
	subscriberLock sync.RWMutex
	subscribers    map[chan *Event]struct{}
	machine        Machine
	containerGroup sync.WaitGroup
	statsCollector *statsCollector
}

// Stop closes all tasks and sends a SIGTERM to each container's pid1 then waits for they to
// terminate.  After it has handled all the SIGCHILD events it will close the signals chan
// and exit.  Stop is a non-blocking call and will return after the containers have been signaled
func (s *Supervisor) Stop(sig chan os.Signal) {
	// Close the tasks channel so that no new containers get started
	close(s.tasks)
	// send a SIGTERM to all containers
	for id, i := range s.containers {
		c := i.container
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

// Events returns an event channel that external consumers can use to receive updates
// on container events
func (s *Supervisor) Events() chan *Event {
	s.subscriberLock.Lock()
	defer s.subscriberLock.Unlock()
	c := make(chan *Event, DefaultBufferSize)
	EventSubscriberCounter.Inc(1)
	s.subscribers[c] = struct{}{}
	return c
}

// Unsubscribe removes the provided channel from receiving any more events
func (s *Supervisor) Unsubscribe(sub chan *Event) {
	s.subscriberLock.Lock()
	defer s.subscriberLock.Unlock()
	delete(s.subscribers, sub)
	close(sub)
	EventSubscriberCounter.Dec(1)
}

// notifySubscribers will send the provided event to the external subscribers
// of the events channel
func (s *Supervisor) notifySubscribers(e *Event) {
	s.subscriberLock.RLock()
	defer s.subscriberLock.RUnlock()
	for sub := range s.subscribers {
		// do a non-blocking send for the channel
		select {
		case sub <- e:
		default:
			logrus.WithField("event", e.Type).Warn("event not sent to subscriber")
		}
	}
}

// Start is a non-blocking call that runs the supervisor for monitoring contianer processes and
// executing new containers.
//
// This event loop is the only thing that is allowed to modify state of containers and processes
// therefore it is save to do operations in the handlers that modify state of the system or
// state of the Supervisor
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

// getContainerForPid returns the container where the provided pid is the pid1 or main
// process in the container
func (s *Supervisor) getContainerForPid(pid int) (runtime.Container, error) {
	for _, i := range s.containers {
		container := i.container
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

// SendEvent sends the provided event the the supervisors main event loop
func (s *Supervisor) SendEvent(evt *Event) {
	s.events <- evt
}

func (s *Supervisor) copyIO(stdin, stdout, stderr string, i *runtime.IO) (*copier, error) {
	config := &ioConfig{
		Stdin:      i.Stdin,
		Stdout:     i.Stdout,
		Stderr:     i.Stderr,
		StdoutPath: stdout,
		StderrPath: stderr,
		StdinPath:  stdin,
	}
	l, err := newCopier(config)
	if err != nil {
		return nil, err
	}
	return l, nil
}
