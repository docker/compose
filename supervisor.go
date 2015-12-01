package containerd

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/linux"
	"github.com/docker/containerd/runtime"
	"github.com/opencontainers/runc/libcontainer"
)

// NewSupervisor returns an initialized Process supervisor.
func NewSupervisor(stateDir string, concurrency int) (*Supervisor, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, err
	}
	// register counters
	r, err := linux.NewRuntime(stateDir)
	if err != nil {
		return nil, err
	}
	j, err := newJournal(filepath.Join(stateDir, "journal.json"))
	if err != nil {
		return nil, err
	}
	s := &Supervisor{
		stateDir:   stateDir,
		containers: make(map[string]runtime.Container),
		processes:  make(map[int]runtime.Container),
		runtime:    r,
		tasks:      make(chan *startTask, concurrency*100),
		journal:    j,
	}
	// register default event handlers
	s.handlers = map[EventType]Handler{
		ExitEventType:            &ExitEvent{s},
		StartContainerEventType:  &StartEvent{s},
		DeleteEventType:          &DeleteEvent{s},
		GetContainerEventType:    &GetContainersEvent{s},
		SignalEventType:          &SignalEvent{s},
		AddProcessEventType:      &AddProcessEvent{s},
		UpdateContainerEventType: &UpdateEvent{s},
	}
	// start the container workers for concurrent container starts
	for i := 0; i < concurrency; i++ {
		s.workerGroup.Add(1)
		go s.startContainerWorker(s.tasks)
	}
	return s, nil
}

type Supervisor struct {
	// stateDir is the directory on the system to store container runtime state information.
	stateDir    string
	containers  map[string]runtime.Container
	processes   map[int]runtime.Container
	handlers    map[EventType]Handler
	runtime     runtime.Runtime
	journal     *journal
	events      chan *Event
	tasks       chan *startTask
	workerGroup sync.WaitGroup
}

// need proper close logic for jobs and stuff so that sending to the channels dont panic
// but can complete jobs
func (s *Supervisor) Close() error {
	return s.journal.Close()
}

func (s *Supervisor) Events() (<-chan *Event, error) {
	return nil, nil
}

// Start is a non-blocking call that runs the supervisor for monitoring contianer processes and
// executing new containers.
//
// This event loop is the only thing that is allowed to modify state of containers and processes.
func (s *Supervisor) Start(events chan *Event) error {
	if events == nil {
		return ErrEventChanNil
	}
	s.events = events
	go func() {
		// allocate an entire thread to this goroutine for the main event loop
		// so that nothing else is scheduled over the top of it.
		goruntime.LockOSThread()
		for e := range events {
			s.journal.write(e)
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
	return nil
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
	EventsCounter.Inc(1)
	s.events <- evt
}

type startTask struct {
	container runtime.Container
	err       chan error
}

func (s *Supervisor) startContainerWorker(tasks chan *startTask) {
	defer s.workerGroup.Done()
	for t := range tasks {
		started := time.Now()
		if err := t.container.Start(); err != nil {
			e := NewEvent(StartContainerEventType)
			e.ID = t.container.ID()
			s.SendEvent(e)
			t.err <- err
			continue
		}
		ContainerStartTimer.UpdateSince(started)
		t.err <- nil
	}
}
