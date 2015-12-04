package containerd

import (
	"sync"
	"time"

	"github.com/docker/containerd/runtime"
)

type Worker interface {
	Start()
}

type Checkpoint struct {
	Path string
	Name string
}

type StartTask struct {
	Container  runtime.Container
	Checkpoint *Checkpoint
	Err        chan error
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
		if t.Checkpoint != nil {
			if err := t.Container.Restore(t.Checkpoint.Path, t.Checkpoint.Name); err != nil {
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
		w.s.containerGroup.Add(1)
		ContainerStartTimer.UpdateSince(started)
		t.Err <- nil
	}
}
