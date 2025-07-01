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

package progress

import (
	"context"
	"fmt"

	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/pkg/api"
)

// NewMixedWriter creates a Writer which allows to mix output from progress.Writer with a api.LogConsumer
func NewMixedWriter(out *streams.Out, consumer api.LogConsumer, dryRun bool) Writer {
	isTerminal := out.IsTerminal()
	if Mode != ModeAuto || !isTerminal {
		return &plainWriter{
			out:    out,
			done:   make(chan bool),
			dryRun: dryRun,
		}
	}
	return &mixedWriter{
		out:    consumer,
		done:   make(chan bool),
		dryRun: dryRun,
	}
}

type mixedWriter struct {
	done   chan bool
	dryRun bool
	out    api.LogConsumer
}

func (p *mixedWriter) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return nil
	}
}

func (p *mixedWriter) Event(e Event) {
	p.out.Status("", fmt.Sprintf("%s %s %s", e.ID, e.Text, SuccessColor(e.StatusText)))
}

func (p *mixedWriter) Events(events []Event) {
	for _, e := range events {
		p.Event(e)
	}
}

func (p *mixedWriter) TailMsgf(msg string, args ...interface{}) {
	msg = fmt.Sprintf(msg, args...)
	p.out.Status("", WarningColor(msg))
}

func (p *mixedWriter) Stop() {
	p.done <- true
}
