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

func (a *allowListLogConsumer) Log(name, service, container, message string) {
	if a.allowList[service] {
		a.delegate.Log(name, service, container, message)
	}
}

func (a *allowListLogConsumer) Status(name, container, message string) {
	if a.allowList[name] {
		a.delegate.Status(name, container, message)
	}
}

func (a *allowListLogConsumer) Register(name string, source string) {
	if a.allowList[name] {
		a.delegate.Register(name, source)
	}
}

// GetWriter creates a io.Writer that will actually split by line and format by LogConsumer
func GetWriter(name, service, container string, events compose.ContainerEventListener) io.Writer {
	return &splitBuffer{
		buffer:    bytes.Buffer{},
		name:      name,
		service:   service,
		container: container,
		consumer:  events,
	}
}

type splitBuffer struct {
	buffer    bytes.Buffer
	name      string
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
			Type:    compose.ContainerEventLog,
			Name:    s.name,
			Service: s.service,
			Source:  s.container,
			Line:    string(line[:len(line)-1]),
		})
	}
	return n, nil
}
