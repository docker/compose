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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	streamsv1 "github.com/docker/compose-cli/cli/server/protos/streams/v1"
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
	err = anypb.UnmarshalTo(a, &m, proto.UnmarshalOptions{})
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

	m, err := anypb.New(&message)
	if err != nil {
		return 0, err
	}

	return len(message.Value), io.Stream.SendMsg(m)
}
