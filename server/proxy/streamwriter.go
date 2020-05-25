package proxy

import (
	"io"

	v1 "github.com/docker/api/protos/containers/v1"
)

type streamWriter struct {
	stream v1.Containers_LogsServer
}

func newStreamWriter(stream v1.Containers_LogsServer) io.Writer {
	return &streamWriter{
		stream: stream,
	}
}

func (w *streamWriter) Write(p []byte) (n int, err error) {
	return len(p), w.stream.Send(&v1.LogsResponse{
		Logs: p,
	})
}
