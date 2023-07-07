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
)

type Writer interface {
	io.Writer
	Flush() string
}

// GetWriter creates a io.Writer that will actually split by line and format by LogConsumer
func GetWriter(consumer func(string)) Writer {
	return &splitWriter{
		buffer:   bytes.Buffer{},
		consumer: consumer,
	}
}

type splitWriter struct {
	buffer   bytes.Buffer
	consumer func(string)
}

// Write implements io.Writer. joins all input, splits on the separator and yields each chunk
func (s *splitWriter) Write(b []byte) (int, error) {
	n, err := s.buffer.Write(b)
	if err != nil {
		return n, err
	}
	for {
		b = s.buffer.Bytes()
		index := bytes.Index(b, []byte{'\n'})
		if index > 0 {
			line := s.buffer.Next(index + 1)
			s.consumer(string(line[:len(line)-1]))
		} else {
			line := s.buffer.String()
			s.buffer.Reset()
			if len(line) > 0 {
				s.consumer(line)
			}
			break
		}
	}
	return n, nil
}

func (s *splitWriter) Flush() string {
	b := s.buffer.Bytes()
	if len(b) == 0 {
		return ""
	}
	return string(b)
}
