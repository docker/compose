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

// Package logstream streams the output of the containers of a project to the
// user: logs, attach, and the monitor of the containers' events they follow.
package logstream

import (
	"github.com/docker/cli/cli/command"

	"github.com/docker/compose/v5/internal/engine"
	"github.com/docker/compose/v5/internal/naming"
	"github.com/docker/compose/v5/pkg/api"
)

// Backend is what the operations need from the engine they stream from.
//
// The terminal reaches them through CLI: the stream attach prints its
// announcement to, and the Docker CLI's own attach implementation.
type Backend interface {
	engine.Backend
	// CLI is the Docker CLI Compose runs in, and the owner of the terminal streams.
	CLI() command.Cli
}

// Service streams the containers' output on a Backend.
type Service struct {
	backend Backend
}

// New creates a Service streaming from the given Backend.
func New(backend Backend) *Service {
	return &Service{backend: backend}
}

func oneOffFilter(b bool) string {
	v := "False"
	if b {
		v = "True"
	}
	return naming.LabelFilter(api.OneoffLabel, v)
}
