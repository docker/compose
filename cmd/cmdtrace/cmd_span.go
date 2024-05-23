/*
   Copyright 2023 Docker Compose CLI authors

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

package cmdtrace

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	dockercli "github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command"
	commands "github.com/docker/compose/v2/cmd/compose"
	"github.com/docker/compose/v2/internal/tracing"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Setup should be called as part of the command's PersistentPreRunE
// as soon as possible after initializing the dockerCli.
//
// It initializes the tracer for the CLI using both auto-detection
// from the Docker context metadata as well as standard OTEL_ env
// vars, creates a root span for the command, and wraps the actual
// command invocation to ensure the span is properly finalized and
// exported before exit.
func Setup(cmd *cobra.Command, dockerCli command.Cli, args []string) error {
	tracingShutdown, err := tracing.InitTracing(dockerCli)
	if err != nil {
		return fmt.Errorf("initializing tracing: %w", err)
	}

	ctx := cmd.Context()
	ctx, cmdSpan := otel.Tracer("").Start(
		ctx,
		"cli/"+strings.Join(commandName(cmd), "-"),
	)
	cmdSpan.SetAttributes(attribute.StringSlice("cli.args", args))
	cmdSpan.SetAttributes(attribute.StringSlice("cli.flags", getFlags(cmd.Flags())))

	cmd.SetContext(ctx)
	wrapRunE(cmd, cmdSpan, tracingShutdown)
	return nil
}

// wrapRunE injects a wrapper function around the command's actual RunE (or Run)
// method. This is necessary to capture the command result for reporting as well
// as flushing any spans before exit.
//
// Unfortunately, PersistentPostRun(E) can't be used for this purpose because it
// only runs if RunE does _not_ return an error, but this should run unconditionally.
func wrapRunE(c *cobra.Command, cmdSpan trace.Span, tracingShutdown tracing.ShutdownFunc) {
	origRunE := c.RunE
	if origRunE == nil {
		origRun := c.Run
		//nolint:unparam // wrapper function for RunE, always returns nil by design
		origRunE = func(cmd *cobra.Command, args []string) error {
			origRun(cmd, args)
			return nil
		}
		c.Run = nil
	}

	c.RunE = func(cmd *cobra.Command, args []string) error {
		cmdErr := origRunE(cmd, args)
		if cmdSpan != nil {
			if cmdErr != nil && !errors.Is(cmdErr, context.Canceled) {
				// default exit code is 1 if a more descriptive error
				// wasn't returned
				exitCode := 1
				var statusErr dockercli.StatusError
				if errors.As(cmdErr, &statusErr) {
					exitCode = statusErr.StatusCode
				}
				cmdSpan.SetStatus(codes.Error, "CLI command returned error")
				cmdSpan.RecordError(cmdErr, trace.WithAttributes(
					attribute.Int("exit_code", exitCode),
				))

			} else {
				cmdSpan.SetStatus(codes.Ok, "")
			}
			cmdSpan.End()
		}
		if tracingShutdown != nil {
			// use background for root context because the cmd's context might have
			// been canceled already
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			// TODO(milas): add an env var to enable logging from the
			// OTel components for debugging purposes
			_ = tracingShutdown(ctx)
		}
		return cmdErr
	}
}

// commandName returns the path components for a given command.
//
// The root Compose command and anything before (i.e. "docker")
// are not included.
//
// For example:
//   - docker compose alpha watch -> [alpha, watch]
//   - docker-compose up -> [up]
func commandName(cmd *cobra.Command) []string {
	var name []string
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == commands.PluginName {
			break
		}
		name = append(name, c.Name())
	}
	sort.Sort(sort.Reverse(sort.StringSlice(name)))
	return name
}

func getFlags(fs *flag.FlagSet) []string {
	var result []string
	fs.Visit(func(flag *flag.Flag) {
		result = append(result, flag.Name)
	})
	return result
}
