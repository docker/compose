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
	"io"

	"google.golang.org/grpc"

	containersv1 "github.com/docker/compose-cli/cli/server/protos/containers/v1"
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
