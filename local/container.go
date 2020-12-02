// +build local

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

package local

import (
	"io"

	moby "github.com/docker/docker/api/types"
)

const (
	containerCreated    = "created"
	containerRestarting = "restarting"
	containerRunning    = "running"
	containerRemoving   = "removing" //nolint
	containerPaused     = "paused"   //nolint
	containerExited     = "exited"   //nolint
	containerDead       = "dead"     //nolint
)

var _ io.ReadCloser = containerStdout{}

type containerStdout struct {
	moby.HijackedResponse
}

func (l containerStdout) Read(p []byte) (n int, err error) {
	return l.Reader.Read(p)
}

func (l containerStdout) Close() error {
	l.HijackedResponse.Close()
	return nil
}

var _ io.WriteCloser = containerStdin{}

type containerStdin struct {
	moby.HijackedResponse
}

func (c containerStdin) Write(p []byte) (n int, err error) {
	return c.Conn.Write(p)
}

func (c containerStdin) Close() error {
	return c.CloseWrite()
}
