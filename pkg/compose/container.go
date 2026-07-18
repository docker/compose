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

package compose

import (
	"io"

	"github.com/moby/moby/client"
)

var _ io.ReadCloser = ContainerStdout{}

// ContainerStdout implements io.ReadCloser for moby.HijackedResponse
type ContainerStdout struct {
	client.HijackedResponse
}

// Read implements io.Reader
func (l ContainerStdout) Read(p []byte) (n int, err error) {
	return l.Reader.Read(p)
}

// Close implements io.Closer
func (l ContainerStdout) Close() error {
	l.HijackedResponse.Close()
	return nil
}

var _ io.WriteCloser = ContainerStdin{}

// ContainerStdin implements io.WriteCloser for moby.HijackedResponse
type ContainerStdin struct {
	client.HijackedResponse
}

// Write implements io.Writer
func (c ContainerStdin) Write(p []byte) (n int, err error) {
	return c.Conn.Write(p)
}

// Close implements io.Closer
func (c ContainerStdin) Close() error {
	return c.CloseWrite()
}
