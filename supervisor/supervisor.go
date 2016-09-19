package supervisor

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
)

const (
	defaultBufferSize = 2048 // size of queue in eventloop
)

// New returns an initialized Process supervisor.
func New(stateDir string, runtimeName, shimName string, runtimeArgs []string, timeout time.Duration, retainCount int) (*Supervisor, error) {
	startTasks := make(chan *startTask, 10)
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
		stateDir:    stateDir,
		containers:  make(map[string]*containerInfo),
		startTasks:  startTasks,
		machine:     machine,
		subscribers: make(map[chan Event]struct{}),
		tasks:       make(chan Task, defaultBufferSize),
		monitor:     monitor,
		runtime:     runtimeName,
		runtimeArgs: runtimeArgs,
		shim:        shimName,
		timeout:     timeout,
	}
	if err := setupEventLog(s, retainCount); err != nil {
		return nil, err
	}
	go s.exitHandler()
	go s.oomHandler()
	if err := s.restore(); err != nil {
		return nil, err
	}
	return s, nil
}

type containerInfo struct {
	container runtime.Container
}

func setupEventLog(s *Supervisor, retainCount int) error {
	if err := readEventLog(s); err != nil {
		return err
	}
	logrus.WithField("count", len(s.eventLog)).Debug("containerd: read past events")
	events := s.Events(time.Time{}, false, "")
	return eventLogger(s, filepath.Join(s.stateDir, "events.log"), events, retainCount)
}

func eventLogger(s *Supervisor, path string, events chan Event, retainCount int) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	go func() {
		var (
			count = len(s.eventLog)
			enc   = json.NewEncoder(f)
		)
		for e := range events {
			// if we have a specified retain count make sure the truncate the event
			// log if it grows past the specified number of events to keep.
			if retainCount > 0 {
				if count > retainCount {
					logrus.Debug("truncating event log")
					// close the log file
					if f != nil {
						f.Close()
					}
					slice := retainCount - 1
					l := len(s.eventLog)
					if slice >= l {
						slice = l
					}
					s.eventLock.Lock()
					s.eventLog = s.eventLog[len(s.eventLog)-slice:]
					s.eventLock.Unlock()
					if f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND|os.O_TRUNC, 0755); err != nil {
						logrus.WithField("error", err).Error("containerd: open event to journal")
						continue
					}
					enc = json.NewEncoder(f)
					count = 0
					for _, le := range s.eventLog {
						if err := enc.Encode(le); err != nil {
							logrus.WithField("error", err).Error("containerd: write event to journal")
						}
					}
				}
			}
			s.eventLock.Lock()
			s.eventLog = append(s.eventLog, e)
			s.eventLock.Unlock()
			count++
			if err := enc.Encode(e); err != nil {
				logrus.WithField("error", err).Error("containerd: write event to journal")
			}
		}
	}()
	return nil
}

func readEventLog(s *Supervisor) error {
	f, err := os.Open(filepath.Join(s.stateDir, "events.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var e Event
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		s.eventLog = append(s.eventLog, e)
	}
	return nil
}

// Supervisor represents a container supervisor
type Supervisor struct {
	// stateDir is the directory on the system to store container runtime state information.
	stateDir string
	// name of the OCI compatible runtime used to execute containers
	runtime     string
	runtimeArgs []string
	shim        string
	containers  map[string]*containerInfo
	startTasks  chan *startTask
	// we need a lock around the subscribers map only because additions and deletions from
	// the map are via the API so we cannot really control the concurrency
	subscriberLock sync.RWMutex
	subscribers    map[chan Event]struct{}
	machine        Machine
	tasks          chan Task
	monitor        *Monitor
	eventLog       []Event
	eventLock      sync.Mutex
	timeout        time.Duration
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
		EventSubscriberCounter.Inc(1)
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
		EventSubscriberCounter.Dec(1)
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
		"stateDir":    s.stateDir,
		"runtime":     s.runtime,
		"runtimeArgs": s.runtimeArgs,
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
	TasksCounter.Inc(1)
	s.tasks <- evt
}

func (s *Supervisor) exitHandler() {
	for p := range s.monitor.Exits() {
		e := &ExitTask{
			Process: p,
		}
		s.SendTask(e)
	}
}

func (s *Supervisor) oomHandler() {
	for id := range s.monitor.OOMs() {
		e := &OOMTask{
			ID: id,
		}
		s.SendTask(e)
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
		container, err := runtime.Load(s.stateDir, id, s.shim, s.timeout)
		if err != nil {
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
		if err := s.monitor.MonitorOOM(container); err != nil && err != runtime.ErrContainerExited {
			logrus.WithField("error", err).Error("containerd: notify OOM events")
		}
		logrus.WithField("id", id).Debug("containerd: container restored")
		var exitedProcesses []runtime.Process
		for _, p := range processes {
			if p.State() == runtime.Running {
				if err := s.monitorProcess(p); err != nil {
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
