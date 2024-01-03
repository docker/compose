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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli"
	"github.com/docker/compose/v2/internal/tracing"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/hashicorp/go-multierror"
)

func (s *composeService) Up(ctx context.Context, project *types.Project, options api.UpOptions) error { //nolint:gocyclo
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

	var eg multierror.Group

	// if we get a second signal during shutdown, we kill the services
	// immediately, so the channel needs to have sufficient capacity or
	// we might miss a signal while setting up the second channel read
	// (this is also why signal.Notify is used vs signal.NotifyContext)
	signalChan := make(chan os.Signal, 2)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	defer close(signalChan)
	var isTerminated bool
	printer := newLogPrinter(options.Start.Attach)

	doneCh := make(chan bool)
	eg.Go(func() error {
		first := true
		gracefulTeardown := func() {
			printer.Cancel()
			fmt.Fprintln(s.stdinfo(), "Gracefully stopping... (press Ctrl+C again to force)")
			eg.Go(func() error {
				err := s.Stop(context.Background(), project.Name, api.StopOptions{
					Services: options.Create.Services,
					Project:  project,
				})
				isTerminated = true
				close(doneCh)
				return err
			})
			first = false
		}
		for {
			select {
			case <-doneCh:
				return nil
			case <-ctx.Done():
				if first {
					gracefulTeardown()
				}
			case <-signalChan:
				if first {
					gracefulTeardown()
				} else {
					eg.Go(func() error {
						return s.Kill(context.Background(), project.Name, api.KillOptions{
							Services: options.Create.Services,
							Project:  project,
						})
					})
					return nil
				}
			}
		}
	})

	var exitCode int
	eg.Go(func() error {
		code, err := printer.Run(options.Start.CascadeStop, options.Start.ExitCodeFrom, func() error {
			fmt.Fprintln(s.stdinfo(), "Aborting on container exit...")
			return progress.Run(ctx, func(ctx context.Context) error {
				return s.Stop(ctx, project.Name, api.StopOptions{
					Services: options.Create.Services,
					Project:  project,
				})
			}, s.stdinfo())
		})
		exitCode = code
		return err
	})

	// We don't use parent (cancelable) context as we manage sigterm to stop the stack
	err = s.start(context.Background(), project.Name, options.Start, printer.HandleEvent)
	if err != nil && !isTerminated { // Ignore error if the process is terminated
		return err
	}

	printer.Stop()

	if !isTerminated {
		// signal for the signal-handler goroutines to stop
		close(doneCh)
	}
	err = eg.Wait().ErrorOrNil()
	if exitCode != 0 {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		return cli.StatusError{StatusCode: exitCode, Status: errMsg}
	}
	return err
}
