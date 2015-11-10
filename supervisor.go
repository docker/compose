package containerd

import (
	"os"
	"sync"

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
	s := &Supervisor{
		stateDir:   stateDir,
		containers: make(map[string]Container),
		runtime:    runtime,
		tasks:      make(chan *startTask, concurrency*100),
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

	runtime Runtime

	events      chan Event
	tasks       chan *startTask
	workerGroup sync.WaitGroup
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
			switch e := evt.(type) {
			case *ExitEvent:
				logrus.WithFields(logrus.Fields{"pid": e.Pid, "status": e.Status}).
					Debug("containerd: process exited")
				container, err := s.getContainerForPid(e.Pid)
				if err != nil {
					if err != errNoContainerForPid {
						logrus.WithField("error", err).Error("containerd: find container for pid")
					}
					continue
				}
				container.SetExited(e.Status)
				if err := s.deleteContainer(container); err != nil {
					logrus.WithField("error", err).Error("containerd: deleting container")
				}
			case *StartContainerEvent:
				container, err := s.runtime.Create(e.ID, e.BundlePath)
				if err != nil {
					e.Err <- err
					continue
				}
				s.containers[e.ID] = container
				s.tasks <- &startTask{
					err:       e.Err,
					container: container,
				}
			case *ContainerStartErrorEvent:
				if container, ok := s.containers[e.ID]; ok {
					if err := s.deleteContainer(container); err != nil {
						logrus.WithField("error", err).Error("containerd: deleting container")
					}
				}
			case *GetContainersEvent:
				for _, c := range s.containers {
					e.Containers = append(e.Containers, c)
				}
				e.Err <- nil
			}
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

func (s *Supervisor) SendEvent(evt Event) {
	s.events <- evt
}

type startTask struct {
	container Container
	err       chan error
}

func (s *Supervisor) startContainerWorker(tasks chan *startTask) {
	defer s.workerGroup.Done()
	for t := range tasks {
		if err := t.container.Start(); err != nil {
			s.SendEvent(&ContainerStartErrorEvent{
				ID: t.container.ID(),
			})
			t.err <- err
			continue
		}
		t.err <- nil
	}
}
