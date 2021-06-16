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

	moby "github.com/docker/docker/api/types"
)

const (
	// ContainerCreated created status
	ContainerCreated = "created"
	// ContainerRestarting restarting status
	ContainerRestarting = "restarting"
	// ContainerRunning running status
	ContainerRunning = "running"
	// ContainerRemoving removing status
	ContainerRemoving = "removing" //nolint
	// ContainerPaused paused status
	ContainerPaused = "paused" //nolint
	// ContainerExited exited status
	ContainerExited = "exited" //nolint
	// ContainerDead dead status
	ContainerDead = "dead" //nolint
)

var _ io.ReadCloser = ContainerStdout{}

// ContainerStdout implement ReadCloser for moby.HijackedResponse
type ContainerStdout struct {
	moby.HijackedResponse
}

// Read implement io.ReadCloser
func (l ContainerStdout) Read(p []byte) (n int, err error) {
	return l.Reader.Read(p)
}

// Close implement io.ReadCloser
func (l ContainerStdout) Close() error {
	l.HijackedResponse.Close()
	return nil
}

var _ io.WriteCloser = ContainerStdin{}

// ContainerStdin implement WriteCloser for moby.HijackedResponse
type ContainerStdin struct {
	moby.HijackedResponse
}

// Write implement io.WriteCloser
func (c ContainerStdin) Write(p []byte) (n int, err error) {
	return c.Conn.Write(p)
}

// Close implement io.WriteCloser
func (c ContainerStdin) Close() error {
	return c.CloseWrite()
}
