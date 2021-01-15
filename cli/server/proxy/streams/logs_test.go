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
	"testing"

	"google.golang.org/grpc/metadata"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	v1 "github.com/docker/compose-cli/cli/server/protos/containers/v1"
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

	assert.NilError(t, err)
	assert.Assert(t, cmp.Len(in, l))
	logs, ok := (ls.logs).(*v1.LogsResponse)
	assert.Assert(t, ok)
	assert.DeepEqual(t, logs.Value, expected.Value)
}
