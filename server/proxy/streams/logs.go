package streams

import (
	"io"

	"google.golang.org/grpc"

	containersv1 "github.com/docker/api/protos/containers/v1"
)

// Log implements an io.Writer that proxies logs over a gRPC stream
type Log struct {
	Stream grpc.ServerStream
}

func newStreamWriter(stream grpc.ServerStream) io.Writer {
	return &Log{
		Stream: stream,
	}
}

func (w *Log) Write(p []byte) (n int, err error) {
	return len(p), w.Stream.SendMsg(&containersv1.LogsResponse{
		Value: p,
	})
}
