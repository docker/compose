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
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/compose/v2/internal/tracing"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Up(ctx context.Context, project *types.Project, options api.UpOptions) error {
	err := progress.Run(ctx, tracing.SpanWrapFunc("project/up", tracing.ProjectOptions(project), func(ctx context.Context) error {
		err := s.create(ctx, project, options.Create)
		if err != nil {
			return err
		}
		if options.Start.Attach == nil {
			return s.start(ctx, project.Name, options.Start, nil)
		}
		return nil
	}), s.stdinfo())
	if err != nil {
		return err
	}

	if options.Start.Attach == nil {
		return err
	}
	if s.dryRun {
		fmt.Fprintln(s.stdout(), "end of 'compose up' output, interactive run is not supported in dry-run mode")
		return err
	}

	printer := newLogPrinter(options.Start.Attach)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	stopFunc := func() error {
		fmt.Fprintln(s.stdinfo(), "Aborting on container exit...")
		ctx := context.Background()
		return progress.Run(ctx, func(ctx context.Context) error {
			go func() {
				<-signalChan
				s.Kill(ctx, project.Name, api.KillOptions{ //nolint:errcheck
					Services: options.Create.Services,
					Project:  project,
				})
			}()

			return s.Stop(ctx, project.Name, api.StopOptions{
				Services: options.Create.Services,
				Project:  project,
			})
		}, s.stdinfo())
	}

	var isTerminated bool
	eg, ctx := errgroup.WithContext(ctx)
	go func() {
		<-signalChan
		isTerminated = true
		printer.Cancel()
		fmt.Fprintln(s.stdinfo(), "Gracefully stopping... (press Ctrl+C again to force)")
		eg.Go(stopFunc)
	}()

	var exitCode int
	eg.Go(func() error {
		code, err := printer.Run(options.Start.CascadeStop, options.Start.ExitCodeFrom, stopFunc)
		exitCode = code
		return err
	})

	err = s.start(ctx, project.Name, options.Start, printer.HandleEvent)
	if err != nil && !isTerminated { // Ignore error if the process is terminated
		return err
	}

	printer.Stop()
	err = eg.Wait()
	if exitCode != 0 {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		return cli.StatusError{StatusCode: exitCode, Status: errMsg}
	}
	return err
}
