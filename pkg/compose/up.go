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
	"sync"
	"syscall"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli"
	"github.com/docker/compose/v2/internal/tracing"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/hashicorp/go-multierror"
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

	// if we get a second signal during shutdown, we kill the services
	// immediately, so the channel needs to have sufficient capacity or
	// we might miss a signal while setting up the second channel read
	// (this is also why signal.Notify is used vs signal.NotifyContext)
	signalChan := make(chan os.Signal, 2)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	signalCancel := sync.OnceFunc(func() {
		signal.Stop(signalChan)
		close(signalChan)
	})
	defer signalCancel()

	printer := newLogPrinter(options.Start.Attach)
	stopFunc := func() error {
		fmt.Fprintln(s.stdinfo(), "Aborting on container exit...")
		ctx := context.Background()
		return progress.Run(ctx, func(ctx context.Context) error {
			// race two goroutines - one that blocks until another signal is received
			// and then does a Kill() and one that immediately starts a friendly Stop()
			errCh := make(chan error, 1)
			go func() {
				if _, ok := <-signalChan; !ok {
					// channel closed, so the outer function is done, which
					// means the other goroutine (calling Stop()) finished
					return
				}
				errCh <- s.Kill(ctx, project.Name, api.KillOptions{
					Services: options.Create.Services,
					Project:  project,
				})
			}()

			go func() {
				errCh <- s.Stop(ctx, project.Name, api.StopOptions{
					Services: options.Create.Services,
					Project:  project,
				})
			}()
			return <-errCh
		}, s.stdinfo())
	}

	var isTerminated bool
	var eg multierror.Group
	eg.Go(func() error {
		if _, ok := <-signalChan; !ok {
			// function finished without receiving a signal
			return nil
		}
		isTerminated = true
		printer.Cancel()
		fmt.Fprintln(s.stdinfo(), "Gracefully stopping... (press Ctrl+C again to force)")
		return stopFunc()
	})

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

	// signal for the goroutines to stop & wait for them to finish any remaining work
	signalCancel()
	printer.Stop()
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
