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

package formatter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/docker/compose-cli/api/compose"
)

// NewLogConsumer creates a new LogConsumer
func NewLogConsumer(ctx context.Context, w io.Writer) compose.LogConsumer {
	return &logConsumer{
		ctx:    ctx,
		colors: map[string]colorFunc{},
		width:  0,
		writer: w,
	}
}

// Log formats a log message as received from service/container
func (l *logConsumer) Log(service, container, message string) {
	if l.ctx.Err() != nil {
		return
	}
	cf, ok := l.colors[service]
	if !ok {
		cf = <-loop
		l.colors[service] = cf
		l.computeWidth()
	}
	prefix := fmt.Sprintf("%-"+strconv.Itoa(l.width)+"s |", container)

	for _, line := range strings.Split(message, "\n") {
		buf := bytes.NewBufferString(fmt.Sprintf("%s %s\n", cf(prefix), line))
		l.writer.Write(buf.Bytes()) // nolint:errcheck
	}
}

func (l *logConsumer) computeWidth() {
	width := 0
	for n := range l.colors {
		if len(n) > width {
			width = len(n)
		}
	}
	l.width = width + 3
}

// LogConsumer consume logs from services and format them
type logConsumer struct {
	ctx    context.Context
	colors map[string]colorFunc
	width  int
	writer io.Writer
}
