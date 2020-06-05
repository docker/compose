package proxy

import (
	"sync"

	"github.com/containerd/containerd/log"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	streamsv1 "github.com/docker/api/protos/streams/v1"
)

// Stream is a bidirectional stream for container IO
type Stream struct {
	streamsv1.Streaming_NewStreamServer

	errm    sync.Mutex
	errChan chan<- error
}

// CloseWithError sends the result of an action to the errChan or nil
// if no erros
func (s *Stream) CloseWithError(err error) error {
	s.errm.Lock()
	defer s.errm.Unlock()

	if s.errChan != nil {
		if err != nil {
			s.errChan <- err
		}
		close(s.errChan)
		s.errChan = nil
	}
	return nil
}

func (p *proxy) NewStream(stream streamsv1.Streaming_NewStreamServer) error {
	var (
		ctx = stream.Context()
		id  = uuid.New().String()
	)
	md := metadata.New(map[string]string{
		"id": id,
	})

	// return the id of the stream to the client
	if err := stream.SendHeader(md); err != nil {
		return err
	}

	errc := make(chan error)

	p.mu.Lock()
	p.streams[id] = &Stream{
		Streaming_NewStreamServer: stream,
		errChan:                   errc,
	}
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		delete(p.streams, id)
		p.mu.Unlock()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		log.G(ctx).Debug("client context canceled")
		return ctx.Err()
	}
}

// io.Reader that forwards everything to the stream
type reader struct {
	stream *Stream
}

func (r reader) Read(p []byte) (int, error) {
	a, err := r.stream.Recv()
	if err != nil {
		return 0, err
	}

	var m streamsv1.BytesMessage
	err = ptypes.UnmarshalAny(a, &m)
	if err != nil {
		return 0, err
	}

	return copy(p, m.Value), nil
}

// io.Writer that writes
type writer struct {
	stream grpc.ServerStream
}

func (w *writer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	message := streamsv1.BytesMessage{
		Type:  streamsv1.IOStream_STDOUT,
		Value: p,
	}

	m, err := ptypes.MarshalAny(&message)
	if err != nil {
		return 0, err
	}

	return len(message.Value), w.stream.SendMsg(m)
}
