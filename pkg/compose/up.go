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

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Up(ctx context.Context, project *types.Project, options api.UpOptions) error {
	err := progress.Run(ctx, func(ctx context.Context) error {
		err := s.create(ctx, project, options.Create)
		if err != nil {
			return err
		}
		if options.Start.Attach == nil {
			return s.start(ctx, project, options.Start, nil)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if options.Start.Attach == nil {
		return err
	}

	printer := newLogPrinter(options.Start.Attach)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	stopFunc := func() error {
		ctx := context.Background()
		return progress.Run(ctx, func(ctx context.Context) error {
			go func() {
				<-signalChan
				s.Kill(ctx, project, api.KillOptions{ // nolint:errcheck
					Services: options.Create.Services,
				})
			}()

			return s.Stop(ctx, project, api.StopOptions{
				Services: options.Create.Services,
			})
		})
	}
	go func() {
		<-signalChan
		printer.Cancel()
		fmt.Println("Gracefully stopping... (press Ctrl+C again to force)")
		stopFunc() // nolint:errcheck
	}()

	var exitCode int
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		code, err := printer.Run(options.Start.CascadeStop, options.Start.ExitCodeFrom, stopFunc)
		exitCode = code
		return err
	})

	err = s.start(ctx, project, options.Start, printer.HandleEvent)
	if err != nil {
		return err
	}

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
