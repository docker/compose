package supervisor

import (
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
)

type Worker interface {
	Start()
}

type StartTask struct {
	Container     runtime.Container
	Checkpoint    string
	Stdin         string
	Stdout        string
	Stderr        string
	Err           chan error
	StartResponse chan StartResponse
}

func NewWorker(s *Supervisor, wg *sync.WaitGroup) Worker {
	return &worker{
		s:  s,
		wg: wg,
	}
}

type worker struct {
	wg *sync.WaitGroup
	s  *Supervisor
}

func (w *worker) Start() {
	defer w.wg.Done()
	for t := range w.s.tasks {
		started := time.Now()
		process, err := t.Container.Start(t.Checkpoint)
		if err != nil {
			evt := NewEvent(DeleteEventType)
			evt.ID = t.Container.ID()
			w.s.SendEvent(evt)
			t.Err <- err
			continue
		}
		/*
		   if w.s.notifier != nil {
		       n, err := t.Container.OOM()
		       if err != nil {
		           logrus.WithField("error", err).Error("containerd: notify OOM events")
		       } else {
		           w.s.notifier.Add(n, t.Container.ID())
		       }
		   }
		*/
		if err := w.s.monitorProcess(process); err != nil {
			logrus.WithField("error", err).Error("containerd: add process to monitor")
		}
		ContainerStartTimer.UpdateSince(started)
		t.Err <- nil
		t.StartResponse <- StartResponse{
			Stdin:  process.Stdin(),
			Stdout: process.Stdout(),
			Stderr: process.Stderr(),
		}
	}
}
