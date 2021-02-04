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

// GetWriter creates a io.Writer that will actually split by line and format by LogConsumer
func GetWriter(service, container string, l compose.LogConsumer) io.Writer {
	return splitBuffer{
		service:   service,
		container: container,
		consumer:  l,
	}
}

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

func (a *allowListLogConsumer) Log(service, container, message string) {
	if a.allowList[service] {
		a.delegate.Log(service, container, message)
	}
}

type splitBuffer struct {
	service   string
	container string
	consumer  compose.LogConsumer
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
