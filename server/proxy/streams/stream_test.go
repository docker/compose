/*
   Copyright 2020 Docker, Inc.

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
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/metadata"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	streamsv1 "github.com/docker/api/protos/streams/v1"
)

type byteStream struct {
	recvResult *any.Any
	recvErr    error

	sendResult interface{}
}

func (bs *byteStream) SetHeader(metadata.MD) error {
	return nil
}

func (bs *byteStream) SendHeader(metadata.MD) error {
	return nil
}

func (bs *byteStream) SetTrailer(metadata.MD) {
}

func (bs *byteStream) Context() context.Context {
	return nil
}

func (bs *byteStream) SendMsg(m interface{}) error {
	bs.sendResult = m
	return nil
}

func (bs *byteStream) Send(*any.Any) error {
	return nil
}

func (bs *byteStream) Recv() (*any.Any, error) {
	return bs.recvResult, bs.recvErr
}

func (bs *byteStream) RecvMsg(m interface{}) error {
	return nil
}

func getReader(t *testing.T, in []byte, errResult error) IO {
	message := streamsv1.BytesMessage{
		Type:  streamsv1.IOStream_STDOUT,
		Value: in,
	}
	m, err := ptypes.MarshalAny(&message)
	require.Nil(t, err)

	return IO{
		Stream: &Stream{
			Streaming_NewStreamServer: &byteStream{
				recvResult: m,
				recvErr:    errResult,
			},
		},
	}
}

func getAny(t *testing.T, in []byte) *any.Any {
	value, err := ptypes.MarshalAny(&streamsv1.BytesMessage{
		Type:  streamsv1.IOStream_STDOUT,
		Value: in,
	})
	require.Nil(t, err)
	return value
}

func TestStreamReader(t *testing.T) {
	in := []byte{104, 101, 108, 108, 111}
	r := getReader(t, in, nil)
	buffer := make([]byte, 5)

	n, err := r.Read(buffer)

	assert.Nil(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, in, buffer)
}

func TestStreamReaderError(t *testing.T) {
	errResult := errors.New("err")
	r := getReader(t, nil, errResult)
	var buffer []byte

	n, err := r.Read(buffer)

	assert.Equal(t, 0, n)
	assert.Equal(t, err, errResult)
}

func TestStreamWriter(t *testing.T) {
	in := []byte{104, 101, 108, 108, 111}
	expected := getAny(t, in)

	bs := byteStream{}
	w := IO{
		Stream: &Stream{
			Streaming_NewStreamServer: &bs,
		},
	}

	n, err := w.Write(in)
	assert.Nil(t, err)
	assert.Equal(t, len(in), n)
	assert.Equal(t, expected, bs.sendResult)
}
