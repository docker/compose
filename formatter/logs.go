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
	"fmt"
	"io"
	"strconv"
	"strings"
)

// NewLogConsumer creates a new LogConsumer
func NewLogConsumer(w io.Writer) LogConsumer {
	return LogConsumer{
		colors: map[string]colorFunc{},
		width:  0,
		writer: w,
	}
}

// Log formats a log message as received from service/container
func (l *LogConsumer) Log(service, container, message string) {
	cf, ok := l.colors[service]
	if !ok {
		cf = <-loop
		l.colors[service] = cf
		l.computeWidth()
	}
	prefix := fmt.Sprintf("%-"+strconv.Itoa(l.width)+"s |", service)

	for _, line := range strings.Split(message, "\n") {
		buf := bytes.NewBufferString(fmt.Sprintf("%s %s\n", cf(prefix), line))
		l.writer.Write(buf.Bytes()) // nolint:errcheck
	}
}

// GetWriter creates a io.Writer that will actually split by line and format by LogConsumer
func (l *LogConsumer) GetWriter(service, container string) io.Writer {
	return splitBuffer{
		service:   service,
		container: container,
		consumer:  l,
	}
}

func (l *LogConsumer) computeWidth() {
	width := 0
	for n := range l.colors {
		if len(n) > width {
			width = len(n)
		}
	}
	l.width = width + 3
}

// LogConsumer consume logs from services and format them
type LogConsumer struct {
	colors map[string]colorFunc
	width  int
	writer io.Writer
}

type splitBuffer struct {
	service   string
	container string
	consumer  *LogConsumer
}

func (s splitBuffer) Write(b []byte) (n int, err error) {
	split := bytes.Split(b, []byte{'\n'})
	for _, line := range split {
		if len(line) != 0 {
			s.consumer.Log(s.service, s.container, string(line))
		}
	}
	return len(b), nil
}
