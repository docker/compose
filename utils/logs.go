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

package utils

import (
	"bytes"
	"io"

	"github.com/docker/compose-cli/api/compose"
)

// FilteredLogConsumer filters logs for given services
func FilteredLogConsumer(consumer compose.LogConsumer, services []string) compose.LogConsumer {
	if len(services) == 0 {
		return consumer
	}
	allowed := map[string]bool{}
	for _, s := range services {
		allowed[s] = true
	}
	return &allowListLogConsumer{
		allowList: allowed,
		delegate:  consumer,
	}
}

type allowListLogConsumer struct {
	allowList map[string]bool
	delegate  compose.LogConsumer
}

func (a *allowListLogConsumer) Log(container, service, message string) {
	if a.allowList[service] {
		a.delegate.Log(container, service, message)
	}
}

func (a *allowListLogConsumer) Status(container, message string) {
	if a.allowList[container] {
		a.delegate.Status(container, message)
	}
}

func (a *allowListLogConsumer) Register(name string) {
	if a.allowList[name] {
		a.delegate.Register(name)
	}
}

// GetWriter creates a io.Writer that will actually split by line and format by LogConsumer
func GetWriter(container, service string, events compose.ContainerEventListener) io.WriteCloser {
	return &splitBuffer{
		buffer:    bytes.Buffer{},
		service:   service,
		container: container,
		consumer:  events,
	}
}

type splitBuffer struct {
	buffer    bytes.Buffer
	service   string
	container string
	consumer  compose.ContainerEventListener
}

// Write implements io.Writer. joins all input, splits on the separator and yields each chunk
func (s *splitBuffer) Write(b []byte) (int, error) {
	n, err := s.buffer.Write(b)
	if err != nil {
		return n, err
	}
	for {
		b = s.buffer.Bytes()
		index := bytes.Index(b, []byte{'\n'})
		if index < 0 {
			break
		}
		line := s.buffer.Next(index + 1)
		s.consumer(compose.ContainerEvent{
			Type:      compose.ContainerEventLog,
			Service:   s.service,
			Container: s.container,
			Line:      string(line[:len(line)-1]),
		})
	}
	return n, nil
}

func (s *splitBuffer) Close() error {
	b := s.buffer.Bytes()
	if len(b) == 0 {
		return nil
	}
	s.consumer(compose.ContainerEvent{
		Type:      compose.ContainerEventLog,
		Service:   s.service,
		Container: s.container,
		Line:      string(b),
	})
	return nil
}
