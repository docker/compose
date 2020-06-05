package proxy

import (
	"io"

	"google.golang.org/grpc"

	containersv1 "github.com/docker/api/protos/containers/v1"
)

type logStream struct {
	stream grpc.ServerStream
}

func newStreamWriter(stream grpc.ServerStream) io.Writer {
	return &logStream{
		stream: stream,
	}
}

func (w *logStream) Write(p []byte) (n int, err error) {
	return len(p), w.stream.SendMsg(&containersv1.LogsResponse{
		Value: p,
	})
}
