package containerd

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/opencontainers/runc/libcontainer"
)

// NewSupervisor returns an initialized Process supervisor.
func NewSupervisor(stateDir string, concurrency int) (*Supervisor, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, err
	}
	// register counters
	runtime, err := NewRuntime(stateDir)
	if err != nil {
		return nil, err
	}
	j, err := newJournal(filepath.Join(stateDir, "journal.json"))
	if err != nil {
		return nil, err
	}
	s := &Supervisor{
		stateDir:   stateDir,
		containers: make(map[string]Container),
		processes:  make(map[int]Container),
		runtime:    runtime,
		tasks:      make(chan *startTask, concurrency*100),
		journal:    j,
	}
	for i := 0; i < concurrency; i++ {
		s.workerGroup.Add(1)
		go s.startContainerWorker(s.tasks)
	}
	return s, nil
}

type Supervisor struct {
	// stateDir is the directory on the system to store container runtime state information.
	stateDir string

	containers map[string]Container

	processes map[int]Container

	runtime Runtime

	journal *journal

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
		runtime.LockOSThread()
		for e := range events {
			s.journal.write(e)
			switch e.Type {
			case ExitEventType:
				logrus.WithFields(logrus.Fields{"pid": e.Pid, "status": e.Status}).
					Debug("containerd: process exited")
				// is it the child process of a container
				if container, ok := s.processes[e.Pid]; ok {
					if err := container.RemoveProcess(e.Pid); err != nil {
						logrus.WithField("error", err).Error("containerd: find container for pid")
					}
					delete(s.processes, e.Pid)
					close(e.Err)
					continue
				}
				// is it the main container's process
				container, err := s.getContainerForPid(e.Pid)
				if err != nil {
					if err != errNoContainerForPid {
						logrus.WithField("error", err).Error("containerd: find container for pid")
					}
					continue
				}
				container.SetExited(e.Status)
				ne := NewEvent(DeleteEventType)
				ne.ID = container.ID()
				s.SendEvent(ne)
			case StartContainerEventType:
				container, err := s.runtime.Create(e.ID, e.BundlePath, e.Stdio)
				if err != nil {
					e.Err <- err
					continue
				}
				s.containers[e.ID] = container
				ContainersCounter.Inc(1)
				s.tasks <- &startTask{
					err:       e.Err,
					container: container,
				}
				continue
			case DeleteEventType:
				if container, ok := s.containers[e.ID]; ok {
					if err := s.deleteContainer(container); err != nil {
						logrus.WithField("error", err).Error("containerd: deleting container")
					}
					ContainersCounter.Dec(1)
				}
			case GetContainerEventType:
				for _, c := range s.containers {
					e.Containers = append(e.Containers, c)
				}
			case SignalEventType:
				container, ok := s.containers[e.ID]
				if !ok {
					e.Err <- ErrContainerNotFound
					continue
				}
				processes, err := container.Processes()
				if err != nil {
					e.Err <- err
					continue
				}
				for _, p := range processes {
					if pid, err := p.Pid(); err == nil && pid == e.Pid {
						e.Err <- p.Signal(e.Signal)
						continue
					}
				}
				e.Err <- ErrProcessNotFound
				continue
			case AddProcessEventType:
				container, ok := s.containers[e.ID]
				if !ok {
					e.Err <- ErrContainerNotFound
					continue
				}
				p, err := s.runtime.StartProcess(container, *e.Process, e.Stdio)
				if err != nil {
					e.Err <- err
					continue
				}
				if e.Pid, err = p.Pid(); err != nil {
					e.Err <- err
					continue
				}
				s.processes[e.Pid] = container
			case UpdateContainerEventType:
				container, ok := s.containers[e.ID]
				if !ok {
					e.Err <- ErrContainerNotFound
					continue
				}
				if e.State.Status != "" {
					switch e.State.Status {
					case Running:
						if err := container.Resume(); err != nil {
							e.Err <- ErrUnknownContainerStatus
							continue
						}
					case Paused:
						if err := container.Pause(); err != nil {
							e.Err <- ErrUnknownContainerStatus
							continue
						}
					default:
						e.Err <- ErrUnknownContainerStatus
						continue
					}
				}
			}
			close(e.Err)
		}
	}()
	return nil
}

func (s *Supervisor) deleteContainer(container Container) error {
	delete(s.containers, container.ID())
	return container.Delete()
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

func (s *Supervisor) SendEvent(evt *Event) {
	s.events <- evt
}

type startTask struct {
	container Container
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
