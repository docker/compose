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

package main

import (
	"context"
	"os"
	"time"

	dockercli "github.com/docker/cli/cli"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/plugin"
	"github.com/docker/cli/cli/command"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/docker/compose/v2/cmd/compatibility"
	commands "github.com/docker/compose/v2/cmd/compose"
	"github.com/docker/compose/v2/internal"
	"github.com/docker/compose/v2/internal/tracing"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
)

func pluginMain() {
	plugin.Run(func(dockerCli command.Cli) *cobra.Command {
		var tracingShutdown tracing.ShutdownFunc
		var cmdSpan trace.Span

		serviceProxy := api.NewServiceProxy().WithService(compose.NewComposeService(dockerCli))
		cmd := commands.RootCommand(dockerCli, serviceProxy)
		originalPreRun := cmd.PersistentPreRunE
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := plugin.PersistentPreRunE(cmd, args); err != nil {
				return err
			}
			// the call to plugin.PersistentPreRunE is what actually
			// initializes the command.Cli instance, so this is the earliest
			// that tracing can be practically initialized (in the future,
			// this could ideally happen in coordination with docker/cli)
			tracingShutdown, _ = tracing.InitTracing(dockerCli)

			ctx := cmd.Context()
			ctx, cmdSpan = tracing.Tracer.Start(
				ctx, "cli/"+cmd.Name(),
				trace.WithAttributes(
					attribute.String("compose.version", internal.Version),
					attribute.String("docker.context", dockerCli.CurrentContext()),
				),
			)
			cmd.SetContext(ctx)

			if originalPreRun != nil {
				return originalPreRun(cmd, args)
			}
			return nil
		}

		// manually wrap RunE instead of using PersistentPostRunE because the
		// latter only runs when RunE does _not_ return an error, but the
		// tracing clean-up logic should always be invoked
		originalPersistentPostRunE := cmd.PersistentPostRunE
		cmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if cmdSpan != nil {
					if err != nil && !errors.Is(err, context.Canceled) {
						cmdSpan.SetStatus(codes.Error, "CLI command returned error")
						cmdSpan.RecordError(err)
					}
					cmdSpan.End()
				}
				if tracingShutdown != nil {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					_ = tracingShutdown(ctx)
				}
			}()
			if originalPersistentPostRunE != nil {
				return originalPersistentPostRunE(cmd, args)
			}
			return nil
		}
		cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
			return dockercli.StatusError{
				StatusCode: compose.CommandSyntaxFailure.ExitCode,
				Status:     err.Error(),
			}
		})
		return cmd
	},
		manager.Metadata{
			SchemaVersion: "0.1.0",
			Vendor:        "Docker Inc.",
			Version:       internal.Version,
		})
}

func main() {
	if plugin.RunningStandalone() {
		os.Args = append([]string{"docker"}, compatibility.Convert(os.Args[1:])...)
	}
	pluginMain()
}
