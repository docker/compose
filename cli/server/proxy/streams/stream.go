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

package streams

import (
	"sync"

	streamsv1 "github.com/docker/compose-cli/cli/server/protos/streams/v1"
)

// Stream is a bidirectional stream for container IO
type Stream struct {
	streamsv1.Streaming_NewStreamServer

	errm    sync.Mutex
	ErrChan chan<- error
}

// CloseWithError sends the result of an action to the errChan or nil
// if no erros
func (s *Stream) CloseWithError(err error) error {
	s.errm.Lock()
	defer s.errm.Unlock()

	if s.ErrChan != nil {
		if err != nil {
			s.ErrChan <- err
		}
		close(s.ErrChan)
		s.ErrChan = nil
	}
	return nil
}
