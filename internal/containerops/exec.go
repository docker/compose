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

package containerops

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command/container"

	"github.com/docker/compose/v5/internal/inspect"
	"github.com/docker/compose/v5/internal/relay"
	"github.com/docker/compose/v5/pkg/api"
)

// isNil reports whether provider is nil, or holds a nil pointer: calling
// ToExecOptions on the latter would dereference it.
func isNil(provider api.ExecOptionsProvider) bool {
	if provider == nil {
		return true
	}
	v := reflect.ValueOf(provider)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// Exec runs a command in a running container of a service and returns its exit code.
func (s *Service) Exec(ctx context.Context, projectName string, provider api.ExecOptionsProvider) (int, error) {
	if isNil(provider) {
		return 0, &api.InvalidOptionError{Option: "options", Reason: "required"}
	}
	options := provider.ToExecOptions()
	projectName = strings.ToLower(projectName)
	target, err := s.backend.SpecifiedContainer(ctx, projectName, inspect.OneOffInclude, false, options.Service, options.Index)
	if err != nil {
		return 0, err
	}
	if err := relay.CheckTarget(target, options.Service, "exec"); err != nil {
		return 0, err
	}

	exec := container.NewExecOptions()
	exec.Interactive = options.Interactive
	exec.TTY = options.Tty
	exec.Detach = options.Detach
	exec.User = options.User
	exec.Privileged = options.Privileged
	exec.Workdir = options.WorkingDir
	exec.Command = options.Command
	for _, v := range options.Environment {
		err := exec.Env.Set(v)
		if err != nil {
			return 0, err
		}
	}

	err = container.RunExec(ctx, s.backend.CLI(), target.ID, exec)
	var sterr cli.StatusError
	if errors.As(err, &sterr) {
		return sterr.StatusCode, err
	}
	return 0, err
}
