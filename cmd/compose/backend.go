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
	"time"

	"github.com/docker/cli/cli/command"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
)

// withBackend creates a compose backend and passes it to fn.
func withBackend(dockerCli command.Cli, opts *BackendOptions, fn func(api.Compose) error) error {
	backend, err := compose.NewComposeService(dockerCli, opts.Options...)
	if err != nil {
		return err
	}
	return fn(backend)
}

// optionalTimeout converts an integer timeout (in seconds) into a *time.Duration.
// If changed is false, nil is returned (no timeout was explicitly set).
func optionalTimeout(t int, changed bool) *time.Duration {
	if !changed {
		return nil
	}
	d := time.Duration(t) * time.Second
	return &d
}
