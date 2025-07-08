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
	"context"
	"os/exec"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli/context/docker"
	"github.com/docker/compose/v2/internal"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// prepareShellOut prepare a shell-out command to be ran by Compose
func (s *composeService) prepareShellOut(gctx context.Context, project *types.Project, cmd *exec.Cmd) error {
	// exec command with same environment Compose is running
	env := types.NewMapping(project.Environment.Values())

	// remove DOCKER_CLI_PLUGIN... variable so a docker-cli plugin will detect it run standalone
	delete(env, manager.ReexecEnvvar)

	// propagate opentelemetry context to child process, see https://github.com/open-telemetry/oteps/blob/main/text/0258-env-context-baggage-carriers.md
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(gctx, &carrier)
	env.Merge(types.Mapping(carrier))

	env["DOCKER_CONTEXT"] = s.dockerCli.CurrentContext()
	env["USER_AGENT"] = "compose/" + internal.Version

	metadata, err := s.dockerCli.ContextStore().GetMetadata(s.dockerCli.CurrentContext())
	if err != nil {
		return err
	}
	endpoint, err := docker.EndpointFromContext(metadata)
	if err != nil {
		return err
	}
	actualHost := s.dockerCli.DockerEndpoint().Host
	if endpoint.Host != actualHost {
		// We are running with `--host` or `DOCKER_HOST` which overrides selected context
		env["DOCKER_HOST"] = actualHost
	}

	cmd.Env = env.Values()
	return nil
}
