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
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/socket"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/sync/errgroup"
)

type external struct {
	Type    string
	Options map[string]string
}

func (s *composeService) runPlugin(ctx context.Context, project *types.Project, service types.ServiceConfig, x external, command string) error {
	plugin, err := manager.GetPlugin(x.Type, s.dockerCli, &cobra.Command{})
	if err != nil {
		return err
	}

	args := []string{"compose", command}
	for k, v := range x.Options {
		args = append(args, fmt.Sprintf("--%s=%s", k, v))
	}
	cmd := exec.CommandContext(ctx, plugin.Path, args...)
	// Remove DOCKER_CLI_PLUGIN... variable so plugin can detect it run standalone
	cmd.Env = filter(os.Environ(), manager.ReexecEnvvar)

	// Use docker/cli mechanism to propagate termination signal to child process
	server, err := socket.NewPluginServer(nil)
	if err != nil {
		defer server.Close() //nolint:errcheck
		cmd.Cancel = server.Close
		cmd.Env = replace(cmd.Env, socket.EnvKey, server.Addr().String())
	}

	cmd.Env = append(cmd.Env, fmt.Sprintf("DOCKER_CONTEXT=%s", s.dockerCli.CurrentContext()))

	// propagate opentelemetry context to child process, see https://github.com/open-telemetry/oteps/blob/main/text/0258-env-context-baggage-carriers.md
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, &carrier)
	cmd.Env = append(cmd.Env, types.Mapping(carrier).Values()...)

	var variables []string
	eg := errgroup.Group{}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return err
	}
	eg.Go(cmd.Wait)

	scanner := bufio.NewScanner(out)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		variables = append(variables, line)
	}

	err = eg.Wait()
	if err != nil {
		return err
	}

	for name, service := range project.Services {
		if _, ok := service.DependsOn[service.Name]; ok {
			service.Environment.OverrideBy(types.NewMappingWithEquals(variables))
			project.Services[name] = service
		}
	}
	return nil
}
