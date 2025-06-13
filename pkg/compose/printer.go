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

	"github.com/docker/compose/v2/pkg/api"
)

// logPrinter watch application containers and collect their logs
type logPrinter interface {
	HandleEvent(event api.ContainerEvent)
}

type printer struct {
	consumer api.LogConsumer
}

// newLogPrinter builds a LogPrinter passing containers logs to LogConsumer
func newLogPrinter(consumer api.LogConsumer) logPrinter {
	printer := printer{
		consumer: consumer,
	}
	return &printer
}

func (p *printer) HandleEvent(event api.ContainerEvent) {
	switch event.Type {
	case api.ContainerEventExited:
		p.consumer.Status(event.Source, fmt.Sprintf("exited with code %d", event.ExitCode))
	case api.ContainerEventRecreated:
		p.consumer.Status(event.Container.Labels[api.ContainerReplaceLabel], "has been recreated")
	case api.ContainerEventLog, api.HookEventLog:
		p.consumer.Log(event.Source, event.Line)
	case api.ContainerEventErr:
		p.consumer.Err(event.Source, event.Line)
	}
}
