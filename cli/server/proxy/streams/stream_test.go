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
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/golang/protobuf/ptypes/any"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	streamsv1 "github.com/docker/compose-cli/cli/server/protos/streams/v1"
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
	m, err := anypb.New(&message)
	assert.NilError(t, err)

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
	value, err := anypb.New(&streamsv1.BytesMessage{
		Type:  streamsv1.IOStream_STDOUT,
		Value: in,
	})
	assert.NilError(t, err)
	return value
}

func TestStreamReader(t *testing.T) {
	in := []byte{104, 101, 108, 108, 111}
	r := getReader(t, in, nil)
	buffer := make([]byte, 5)

	n, err := r.Read(buffer)

	assert.NilError(t, err)
	assert.Equal(t, n, 5)
	assert.DeepEqual(t, buffer, in)
}

func TestStreamReaderError(t *testing.T) {
	errResult := errors.New("err")
	r := getReader(t, nil, errResult)
	var buffer []byte

	n, err := r.Read(buffer)

	assert.Equal(t, n, 0)
	assert.Error(t, err, errResult.Error())
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
	assert.NilError(t, err)
	assert.Assert(t, cmp.Len(in, n))
	sendResult, ok := (bs.sendResult).(*anypb.Any)
	assert.Assert(t, ok)
	assert.DeepEqual(t, sendResult.Value, expected.Value)
}
