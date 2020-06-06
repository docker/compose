package streams

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/metadata"

	v1 "github.com/docker/api/protos/containers/v1"
)

type logServer struct {
	logs interface{}
}

func (ls *logServer) Send(response *v1.LogsResponse) error {
	return nil
}

func (ls *logServer) SetHeader(metadata.MD) error {
	return nil
}

func (ls *logServer) SendHeader(metadata.MD) error {
	return nil
}

func (ls *logServer) SetTrailer(metadata.MD) {
}

func (ls *logServer) Context() context.Context {
	return nil
}

func (ls *logServer) SendMsg(m interface{}) error {
	ls.logs = m
	return nil
}

func (ls *logServer) RecvMsg(m interface{}) error {
	return nil
}

func TestLogStreamWriter(t *testing.T) {
	ls := &logServer{}
	sw := newStreamWriter(ls)
	in := []byte{104, 101, 108, 108, 111}
	expected := &v1.LogsResponse{
		Value: in,
	}

	l, err := sw.Write(in)

	assert.Nil(t, err)
	assert.Equal(t, len(in), l)
	assert.Equal(t, expected, ls.logs)
}
