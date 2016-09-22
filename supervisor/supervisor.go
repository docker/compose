package supervisor

import (
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/monitor"
	"github.com/docker/containerd/runtime"
)

const (
	defaultBufferSize = 2048 // size of queue in eventloop
)

type Config struct {
	StateDir         string
	Runtime          string
	ShimName         string
	RuntimeArgs      []string
	Timeout          time.Duration
	EventRetainCount int
}

// New returns an initialized Process supervisor.
func New(c Config) (*Supervisor, error) {
	startTasks := make(chan *startTask, 10)
	if err := os.MkdirAll(c.StateDir, 0755); err != nil {
		return nil, err
	}
	machine, err := CollectMachineInformation()
	if err != nil {
		return nil, err
	}
	m, err := monitor.New()
	if err != nil {
		return nil, err
	}
	go m.Run()
	s := &Supervisor{
		config:      c,
		containers:  make(map[string]*containerInfo),
		startTasks:  startTasks,
		machine:     machine,
		subscribers: make(map[chan Event]struct{}),
		tasks:       make(chan Task, defaultBufferSize),
		monitor:     m,
	}
	if err := setupEventLog(s, c.EventRetainCount); err != nil {
		return nil, err
	}
	go s.monitorEventHandler()
	if err := s.restore(); err != nil {
		return nil, err
	}
	return s, nil
}

// Supervisor represents a container supervisor
type Supervisor struct {
	config     Config
	containers map[string]*containerInfo
	startTasks chan *startTask
	// we need a lock around the subscribers map only because additions and deletions from
	// the map are via the API so we cannot really control the concurrency
	subscriberLock sync.RWMutex
	subscribers    map[chan Event]struct{}
	machine        Machine
	tasks          chan Task
	monitor        *monitor.Monitor
	eventLog       []Event
	eventLock      sync.Mutex
}

// Stop closes all startTasks and sends a SIGTERM to each container's pid1 then waits for they to
// terminate.  After it has handled all the SIGCHILD events it will close the signals chan
// and exit.  Stop is a non-blocking call and will return after the containers have been signaled
func (s *Supervisor) Stop() {
	// Close the startTasks channel so that no new containers get started
	close(s.startTasks)
}

// Close closes any open files in the supervisor but expects that Stop has been
// callsed so that no more containers are started.
func (s *Supervisor) Close() error {
	return nil
}

// Event represents a container event
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	PID       string    `json:"pid,omitempty"`
	Status    uint32    `json:"status,omitempty"`
}

// Events returns an event channel that external consumers can use to receive updates
// on container events
func (s *Supervisor) Events(from time.Time, storedOnly bool, id string) chan Event {
	c := make(chan Event, defaultBufferSize)
	if storedOnly {
		defer s.Unsubscribe(c)
	}
	s.subscriberLock.Lock()
	defer s.subscriberLock.Unlock()
	if !from.IsZero() {
		// replay old event
		s.eventLock.Lock()
		past := s.eventLog[:]
		s.eventLock.Unlock()
		for _, e := range past {
			if e.Timestamp.After(from) {
				if id == "" || e.ID == id {
					c <- e
				}
			}
		}
	}
	if storedOnly {
		close(c)
	} else {
		s.subscribers[c] = struct{}{}
	}
	return c
}

// Unsubscribe removes the provided channel from receiving any more events
func (s *Supervisor) Unsubscribe(sub chan Event) {
	s.subscriberLock.Lock()
	defer s.subscriberLock.Unlock()
	if _, ok := s.subscribers[sub]; ok {
		delete(s.subscribers, sub)
		close(sub)
	}
}

// notifySubscribers will send the provided event to the external subscribers
// of the events channel
func (s *Supervisor) notifySubscribers(e Event) {
	s.subscriberLock.RLock()
	defer s.subscriberLock.RUnlock()
	for sub := range s.subscribers {
		// do a non-blocking send for the channel
		select {
		case sub <- e:
		default:
			logrus.WithField("event", e.Type).Warn("containerd: event not sent to subscriber")
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
		"stateDir":    s.config.StateDir,
		"runtime":     s.config.Runtime,
		"runtimeArgs": s.config.RuntimeArgs,
		"memory":      s.machine.Memory,
		"cpus":        s.machine.Cpus,
	}).Debug("containerd: supervisor running")
	go func() {
		for i := range s.tasks {
			s.handleTask(i)
		}
	}()
	return nil
}

// Machine returns the machine information for which the
// supervisor is executing on.
func (s *Supervisor) Machine() Machine {
	return s.machine
}

// SendTask sends the provided event the the supervisors main event loop
func (s *Supervisor) SendTask(evt Task) {
	s.tasks <- evt
}

func (s *Supervisor) monitorEventHandler() {
	for e := range s.monitor.Events() {
		switch t := e.(type) {
		case runtime.Process:
			if err := s.monitor.Remove(e); err != nil {
				logrus.WithField("error", err).Error("containerd: remove process event FD from monitor")
			}
			if err := t.Close(); err != nil {
				logrus.WithField("error", err).Error("containerd: close process event FD")
			}
			ev := &ExitTask{
				Process: t,
			}
			s.SendTask(ev)
		case runtime.OOM:
			if t.Removed() {
				if err := s.monitor.Remove(e); err != nil {
					logrus.WithField("error", err).Error("containerd: remove oom event FD from monitor")
				}
				if err := t.Close(); err != nil {
					logrus.WithField("error", err).Error("containerd: close oom event FD")
				}
				// don't send an event on the close of this FD
				continue
			}
			ev := &OOMTask{
				ID: t.ContainerID(),
			}
			s.SendTask(ev)
		}
	}
}

func (s *Supervisor) restore() error {
	dirs, err := ioutil.ReadDir(s.config.StateDir)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		id := d.Name()
		container, err := runtime.Load(s.config.StateDir, id, s.config.ShimName, s.config.Timeout)
		if err != nil {
			return err
		}
		processes, err := container.Processes()
		if err != nil {
			return err
		}

		s.containers[id] = &containerInfo{
			container: container,
		}
		oom, err := container.OOM()
		if err != nil {
			logrus.WithField("error", err).Error("containerd: get oom FD")
		}
		if err := s.monitor.Add(oom); err != nil && err != runtime.ErrContainerExited {
			logrus.WithField("error", err).Error("containerd: notify OOM events")
		}
		logrus.WithField("id", id).Debug("containerd: container restored")
		var exitedProcesses []runtime.Process
		for _, p := range processes {
			if p.State() == runtime.Running {
				if err := s.monitor.Add(p); err != nil {
					return err
				}
			} else {
				exitedProcesses = append(exitedProcesses, p)
			}
		}
		if len(exitedProcesses) > 0 {
			// sort processes so that init is fired last because that is how the kernel sends the
			// exit events
			sortProcesses(exitedProcesses)
			for _, p := range exitedProcesses {
				e := &ExitTask{
					Process: p,
				}
				s.SendTask(e)
			}
		}
	}
	return nil
}

func (s *Supervisor) handleTask(i Task) {
	var err error
	switch t := i.(type) {
	case *AddProcessTask:
		err = s.addProcess(t)
	case *CreateCheckpointTask:
		err = s.createCheckpoint(t)
	case *DeleteCheckpointTask:
		err = s.deleteCheckpoint(t)
	case *StartTask:
		err = s.start(t)
	case *DeleteTask:
		err = s.delete(t)
	case *ExitTask:
		err = s.exit(t)
	case *GetContainersTask:
		err = s.getContainers(t)
	case *SignalTask:
		err = s.signal(t)
	case *StatsTask:
		err = s.stats(t)
	case *UpdateTask:
		err = s.updateContainer(t)
	case *UpdateProcessTask:
		err = s.updateProcess(t)
	case *OOMTask:
		err = s.oom(t)
	default:
		err = ErrUnknownTask
	}
	if err != errDeferredResponse {
		i.ErrorCh() <- err
		close(i.ErrorCh())
	}
}
