package supervisor

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/chanotify"
	"github.com/docker/containerd/eventloop"
	"github.com/docker/containerd/runtime"
)

const (
	statsInterval     = 1 * time.Second
	defaultBufferSize = 2048 // size of queue in eventloop
)

// New returns an initialized Process supervisor.
func New(stateDir string, tasks chan *StartTask, oom bool) (*Supervisor, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, err
	}
	machine, err := CollectMachineInformation()
	if err != nil {
		return nil, err
	}
	monitor, err := NewMonitor()
	if err != nil {
		return nil, err
	}
	s := &Supervisor{
		stateDir:       stateDir,
		containers:     make(map[string]*containerInfo),
		tasks:          tasks,
		machine:        machine,
		subscribers:    make(map[chan *Event]struct{}),
		statsCollector: newStatsCollector(statsInterval),
		el:             eventloop.NewChanLoop(defaultBufferSize),
		monitor:        monitor,
	}
	if oom {
		s.notifier = chanotify.New()
		go func() {
			for id := range s.notifier.Chan() {
				e := NewEvent(OOMEventType)
				e.ID = id.(string)
				s.SendEvent(e)
			}
		}()
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
		StopStatsEventType:        &StopStatsEvent{s},
	}
	go s.exitHandler()
	if err := s.restore(); err != nil {
		return nil, err
	}
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
	handlers   map[EventType]Handler
	events     chan *Event
	tasks      chan *StartTask
	// we need a lock around the subscribers map only because additions and deletions from
	// the map are via the API so we cannot really control the concurrency
	subscriberLock sync.RWMutex
	subscribers    map[chan *Event]struct{}
	machine        Machine
	statsCollector *statsCollector
	notifier       *chanotify.Notifier
	el             eventloop.EventLoop
	monitor        *Monitor
}

// Stop closes all tasks and sends a SIGTERM to each container's pid1 then waits for they to
// terminate.  After it has handled all the SIGCHILD events it will close the signals chan
// and exit.  Stop is a non-blocking call and will return after the containers have been signaled
func (s *Supervisor) Stop() {
	// Close the tasks channel so that no new containers get started
	close(s.tasks)
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
	c := make(chan *Event, defaultBufferSize)
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
	logrus.WithFields(logrus.Fields{
		"stateDir": s.stateDir,
	}).Debug("Supervisor started")
	return s.el.Start()
}

// Machine returns the machine information for which the
// supervisor is executing on.
func (s *Supervisor) Machine() Machine {
	return s.machine
}

// SendEvent sends the provided event the the supervisors main event loop
func (s *Supervisor) SendEvent(evt *Event) {
	EventsCounter.Inc(1)
	s.el.Send(&commonEvent{data: evt, sv: s})
}

func (s *Supervisor) exitHandler() {
	for p := range s.monitor.Exits() {
		e := NewEvent(ExitEventType)
		e.Process = p
		s.SendEvent(e)
	}
}

func (s *Supervisor) monitorProcess(p runtime.Process) error {
	return s.monitor.Monitor(p)
}

func (s *Supervisor) restore() error {
	dirs, err := ioutil.ReadDir(s.stateDir)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		id := d.Name()
		container, err := runtime.Load(s.stateDir, id)
		if err != nil {
			if err == runtime.ErrContainerExited {
				logrus.WithField("id", id).Debug("containerd: container exited while away")
				// TODO: fire events to do the removal
				if err := os.RemoveAll(filepath.Join(s.stateDir, id)); err != nil {
					logrus.WithField("error", err).Warn("containerd: remove container state")
				}
				continue
			}
			return err
		}
		processes, err := container.Processes()
		if err != nil {
			return err
		}
		ContainersCounter.Inc(1)
		s.containers[id] = &containerInfo{
			container: container,
		}
		logrus.WithField("id", id).Debug("containerd: container restored")
		for _, p := range processes {
			if err := s.monitorProcess(p); err != nil {
				return err
			}
		}
	}
	return nil
}
