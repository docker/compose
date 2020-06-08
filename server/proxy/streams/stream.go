package streams

import (
	"sync"

	streamsv1 "github.com/docker/api/protos/streams/v1"
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
