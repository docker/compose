package streams

import (
	"github.com/golang/protobuf/ptypes"

	streamsv1 "github.com/docker/api/protos/streams/v1"
)

// IO implements an io.ReadWriter that forwards everything to the stream
type IO struct {
	Stream *Stream
}

func (io *IO) Read(p []byte) (int, error) {
	a, err := io.Stream.Recv()
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

func (io *IO) Write(p []byte) (n int, err error) {
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

	return len(message.Value), io.Stream.SendMsg(m)
}
