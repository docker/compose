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
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/docker/compose-cli/api/compose"
)

// NewLogConsumer creates a new LogConsumer
func NewLogConsumer(ctx context.Context, w io.Writer, color bool, prefix bool) compose.LogConsumer {
	return &logConsumer{
		ctx:        ctx,
		presenters: map[string]*Presenter{},
		width:      0,
		writer:     w,
		color:      color,
		prefix:     prefix,
	}
}

func (l *logConsumer) Register(service string, source string) {
	l.register(service, source)
}

func (l *logConsumer) register(service string, source string) *Presenter {
	cf := monochrome
	if l.color {
		cf = <-loop
	}
	p := &Presenter{
		colors:    cf,
		service:   service,
		container: source,
	}
	l.presenters[source] = p
	if l.prefix {
		l.computeWidth()
		for _, p := range l.presenters {
			p.setPrefix(l.width)
		}
	}
	return p
}

// Log formats a log message as received from service/container
func (l *logConsumer) Log(service, container, message string) {
	if l.ctx.Err() != nil {
		return
	}
	p, ok := l.presenters[container]
	if !ok { // should have been registered, but ¯\_(ツ)_/¯
		p = l.register(service, container)
	}
	for _, line := range strings.Split(message, "\n") {
		fmt.Fprintf(l.writer, "%s %s\n", p.prefix, line) // nolint:errcheck
	}
}

func (l *logConsumer) Status(service, container, msg string) {
	p, ok := l.presenters[container]
	if !ok {
		p = l.register(service, container)
	}
	s := p.colors(fmt.Sprintf("%s %s\n", container, msg))
	l.writer.Write([]byte(s)) // nolint:errcheck
}

func (l *logConsumer) computeWidth() {
	width := 0
	for n := range l.presenters {
		if len(n) > width {
			width = len(n)
		}
	}
	l.width = width + 1
}

// LogConsumer consume logs from services and format them
type logConsumer struct {
	ctx        context.Context
	presenters map[string]*Presenter
	width      int
	writer     io.Writer
	color      bool
	prefix     bool
}

type Presenter struct {
	colors    colorFunc
	service   string
	container string
	prefix    string
}

func (p *Presenter) setPrefix(width int) {
	p.prefix = p.colors(fmt.Sprintf("%-"+strconv.Itoa(width)+"s |", p.container))
}
