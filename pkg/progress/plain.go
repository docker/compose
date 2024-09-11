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
	"io"

	"github.com/docker/compose/v2/pkg/api"
)

type plainWriter struct {
	out    io.Writer
	done   chan bool
	dryRun bool
}

func (p *plainWriter) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return nil
	}
}

func (p *plainWriter) Event(e Event) {
	prefix := ""
	if p.dryRun {
		prefix = api.DRYRUN_PREFIX
	}
	_, _ = fmt.Fprintln(p.out, prefix, e.ID, e.Text, e.StatusText)
}

func (p *plainWriter) Events(events []Event) {
	for _, e := range events {
		p.Event(e)
	}
}

func (p *plainWriter) TailMsgf(msg string, args ...interface{}) {
	msg = fmt.Sprintf(msg, args...)
	if p.dryRun {
		msg = api.DRYRUN_PREFIX + msg
	}
	_, _ = fmt.Fprintln(p.out, msg)
}

func (p *plainWriter) Stop() {
	p.done <- true
}
