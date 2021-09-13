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
	"sync"

	"github.com/docker/compose/v2/pkg/api"
)

// NewLogConsumer creates a new LogConsumer
func NewLogConsumer(ctx context.Context, w io.Writer, color bool, prefix bool) api.LogConsumer {
	return &logConsumer{
		ctx:        ctx,
		presenters: sync.Map{},
		width:      0,
		writer:     w,
		color:      color,
		prefix:     prefix,
	}
}

func (l *logConsumer) Register(name string) {
	l.register(name)
}

func (l *logConsumer) register(name string) *presenter {
	cf := monochrome
	if l.color {
		cf = nextColor()
	}
	p := &presenter{
		colors: cf,
		name:   name,
	}
	l.presenters.Store(name, p)
	if l.prefix {
		l.computeWidth()
		l.presenters.Range(func(key, value interface{}) bool {
			p := value.(*presenter)
			p.setPrefix(l.width)
			return true
		})
	}
	return p
}

func (l *logConsumer) getPresenter(container string) *presenter {
	p, ok := l.presenters.Load(container)
	if !ok { // should have been registered, but ¯\_(ツ)_/¯
		return l.register(container)
	}
	return p.(*presenter)
}

// Log formats a log message as received from name/container
func (l *logConsumer) Log(container, service, message string) {
	if l.ctx.Err() != nil {
		return
	}
	p := l.getPresenter(container)
	for _, line := range strings.Split(message, "\n") {
		fmt.Fprintf(l.writer, "%s %s\n", p.prefix, line) // nolint:errcheck
	}
}

func (l *logConsumer) Status(container, msg string) {
	p := l.getPresenter(container)
	s := p.colors(fmt.Sprintf("%s %s\n", container, msg))
	l.writer.Write([]byte(s)) // nolint:errcheck
}

func (l *logConsumer) computeWidth() {
	width := 0
	l.presenters.Range(func(key, value interface{}) bool {
		p := value.(*presenter)
		if len(p.name) > width {
			width = len(p.name)
		}
		return true
	})
	l.width = width + 1
}

// LogConsumer consume logs from services and format them
type logConsumer struct {
	ctx        context.Context
	presenters sync.Map // map[string]*presenter
	width      int
	writer     io.Writer
	color      bool
	prefix     bool
}

type presenter struct {
	colors colorFunc
	name   string
	prefix string
}

func (p *presenter) setPrefix(width int) {
	p.prefix = p.colors(fmt.Sprintf("%-"+strconv.Itoa(width)+"s |", p.name))
}
