package containerd

import (
	"os"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
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
		processes:  make(map[int]Container),
		containers: make(map[string]Container),
		runtime:    runtime,
		jobs:       make(chan Job, 1024),
	}
	for i := 0; i < concurrency; i++ {
		s.workerGroup.Add(1)
		go s.worker(i)
	}
	return s, nil
}

type Supervisor struct {
	// stateDir is the directory on the system to store container runtime state information.
	stateDir string

	processes  map[int]Container
	containers map[string]Container

	runtime Runtime

	events chan Event
	jobs   chan Job

	workerGroup sync.WaitGroup
}

// Run is a blocking call that runs the supervisor for monitoring contianer processes and
// executing new containers.
//
// This event loop is the only thing that is allowed to modify state of containers and processes.
func (s *Supervisor) Run(events chan Event) error {
	if events == nil {
		return ErrEventChanNil
	}
	s.events = events
	for evt := range events {
		logrus.WithField("event", evt).Debug("containerd: processing event")
		switch e := evt.(type) {
		case *ExitEvent:
			logrus.WithFields(logrus.Fields{
				"pid":    e.Pid,
				"status": e.Status,
			}).Debug("containerd: process exited")
			if container, ok := s.processes[e.Pid]; ok {
				container.SetExited(e.Status)
			}
		case *StartedEvent:
			s.containers[e.ID] = e.Container
		case *CreateContainerEvent:
			j := &CreateJob{
				ID:         e.ID,
				BundlePath: e.BundlePath,
				Err:        e.Err,
			}
			s.jobs <- j
		}
	}
	return nil
}

func (s *Supervisor) SendEvent(evt Event) {
	s.events <- evt
}

// Stop initiates a shutdown of the supervisor killing all processes under supervision.
func (s *Supervisor) Stop() {

}

func (s *Supervisor) worker(id int) {
	defer func() {
		s.workerGroup.Done()
		logrus.WithField("worker", id).Debug("containerd: worker finished")
	}()
	logrus.WithField("worker", id).Debug("containerd: starting worker")
	for job := range s.jobs {
		switch j := job.(type) {
		case *CreateJob:
			start := time.Now()
			container, err := s.runtime.Create(j.ID, j.BundlePath)
			if err != nil {
				j.Err <- err
			}
			s.SendEvent(&StartedEvent{
				ID:        j.ID,
				Container: container,
			})
			j.Err <- nil
			containerStartTimer.UpdateSince(start)
		}
	}
}
