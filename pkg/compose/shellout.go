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
	"os"
	"os/exec"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli-plugins/metadata"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/moby/moby/client"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/docker/compose/v5/internal"
)

// prepareShellOut prepare a shell-out command to be ran by Compose
func (s *composeService) prepareShellOut(gctx context.Context, env types.Mapping, cmd *exec.Cmd) error {
	env = env.Clone()
	// remove DOCKER_CLI_PLUGIN... variable so a docker-cli plugin will detect it run standalone
	delete(env, metadata.ReexecEnvvar)

	// propagate opentelemetry context to child process, see https://github.com/open-telemetry/oteps/blob/main/text/0258-env-context-baggage-carriers.md
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(gctx, &carrier)
	env.Merge(types.Mapping(carrier))

	cmd.Env = env.Values()
	return nil
}

// propagateDockerEndpoint produces DOCKER_* env vars for a child CLI plugin to target the same docker endpoint
// `cleanup` func MUST be called after child process completion to enforce removal of cert files
func (s *composeService) propagateDockerEndpoint() ([]string, func(), error) {
	cleanup := func() {}
	env := types.Mapping{}

	env[command.EnvOverrideContext] = s.dockerCli.CurrentContext()
	env["USER_AGENT"] = "compose/" + internal.Version

	endpoint := s.dockerCli.DockerEndpoint()
	env[client.EnvOverrideHost] = endpoint.Host
	if endpoint.TLSData != nil {
		certs, err := os.MkdirTemp("", "compose")
		if err != nil {
			return nil, cleanup, err
		}
		cleanup = func() {
			_ = os.RemoveAll(certs)
		}
		env[client.EnvOverrideCertPath] = certs
		env["DOCKER_TLS"] = "1"
		if !endpoint.SkipTLSVerify {
			env[client.EnvTLSVerify] = "1"
		}

		err = os.WriteFile(filepath.Join(certs, flags.DefaultKeyFile), endpoint.TLSData.Key, 0o600)
		if err != nil {
			return nil, cleanup, err
		}
		err = os.WriteFile(filepath.Join(certs, flags.DefaultCertFile), endpoint.TLSData.Cert, 0o600)
		if err != nil {
			return nil, cleanup, err
		}
		err = os.WriteFile(filepath.Join(certs, flags.DefaultCaFile), endpoint.TLSData.CA, 0o600)
		if err != nil {
			return nil, cleanup, err
		}
	}
	return env.Values(), cleanup, nil
}
