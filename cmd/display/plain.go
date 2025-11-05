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

package display

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/compose/v2/pkg/api"
)

func Plain(out io.Writer) api.EventProcessor {
	return &plainWriter{
		out: out,
	}
}

type plainWriter struct {
	out    io.Writer
	dryRun bool
}

func (p *plainWriter) Start(ctx context.Context, operation string) {
}

func (p *plainWriter) Event(e api.Resource) {
	prefix := ""
	if p.dryRun {
		prefix = api.DRYRUN_PREFIX
	}
	_, _ = fmt.Fprintln(p.out, prefix, e.ID, e.Text, e.Details)
}

func (p *plainWriter) On(events ...api.Resource) {
	for _, e := range events {
		p.Event(e)
	}
}

func (p *plainWriter) Done(_ string, _ bool) {
}
