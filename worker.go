package containerd

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
	IO            *runtime.IO
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
		l, err := w.s.copyIO(t.Stdin, t.Stdout, t.Stderr, t.IO)
		if err != nil {
			evt := NewEvent(DeleteEventType)
			evt.ID = t.Container.ID()
			w.s.SendEvent(evt)
			t.Err <- err
			continue
		}
		w.s.containers[t.Container.ID()].copier = l
		if t.Checkpoint != "" {
			if err := t.Container.Restore(t.Checkpoint); err != nil {
				evt := NewEvent(DeleteEventType)
				evt.ID = t.Container.ID()
				w.s.SendEvent(evt)
				t.Err <- err
				continue
			}
		} else {
			if err := t.Container.Start(); err != nil {
				evt := NewEvent(DeleteEventType)
				evt.ID = t.Container.ID()
				w.s.SendEvent(evt)
				t.Err <- err
				continue
			}
		}
		pid, err := t.Container.Pid()
		if err != nil {
			logrus.WithField("error", err).Error("containerd: get container main pid")
		}
		if w.s.notifier != nil {
			n, err := t.Container.OOM()
			if err != nil {
				logrus.WithField("error", err).Error("containerd: notify OOM events")
			} else {
				w.s.notifier.Add(n, t.Container.ID())
			}
		}
		ContainerStartTimer.UpdateSince(started)
		t.Err <- nil
		t.StartResponse <- StartResponse{
			Pid: pid,
		}
	}
}
