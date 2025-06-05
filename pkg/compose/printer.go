/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package compose

import (
	"fmt"
	"sync"

	"github.com/docker/compose/v2/pkg/api"
)

// logPrinter watch application containers and collect their logs
type logPrinter interface {
	HandleEvent(event api.ContainerEvent)
	Run() error
	Stop()
}

type printer struct {
	queue    chan api.ContainerEvent
	consumer api.LogConsumer
	stopCh   chan struct{} // stopCh is a signal channel for producers to stop sending events to the queue
	stop     sync.Once
}

// newLogPrinter builds a LogPrinter passing containers logs to LogConsumer
func newLogPrinter(consumer api.LogConsumer) logPrinter {
	printer := printer{
		consumer: consumer,
		queue:    make(chan api.ContainerEvent),
		stopCh:   make(chan struct{}),
		stop:     sync.Once{},
	}
	return &printer
}

func (p *printer) Stop() {
	p.stop.Do(func() {
		close(p.stopCh)
		for {
			select {
			case <-p.queue:
				// purge the queue to free producers goroutines
				// p.queue will be garbage collected
			default:
				return
			}
		}
	})
}

func (p *printer) HandleEvent(event api.ContainerEvent) {
	select {
	case <-p.stopCh:
		return
	default:
		p.queue <- event
	}
}

func (p *printer) Run() error {
	defer p.Stop()

	// containers we are tracking. Use true when container is running, false after we receive a stop|die signal
	for {
		select {
		case <-p.stopCh:
			return nil
		case event := <-p.queue:
			switch event.Type {
			case api.ContainerEventExited, api.ContainerEventStopped, api.ContainerEventRecreated, api.ContainerEventRestarted:
				p.consumer.Status(event.Source, fmt.Sprintf("exited with code %d", event.ExitCode))
				if event.Type == api.ContainerEventRecreated {
					p.consumer.Status(event.Source, "has been recreated")
				}
			case api.ContainerEventLog, api.HookEventLog:
				p.consumer.Log(event.Source, event.Line)
			case api.ContainerEventErr:
				p.consumer.Err(event.Source, event.Line)
			}
		}
	}
}
