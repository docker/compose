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
	"sync/atomic"
	"syscall"

	"github.com/compose-spec/compose-go/v2/types"
	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/cli/cli"
	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/internal/tracing"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/eiannone/keyboard"
	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
)

func (s *composeService) Up(ctx context.Context, project *types.Project, options api.UpOptions) error { //nolint:gocyclo
	err := progress.Run(ctx, tracing.SpanWrapFunc("project/up", tracing.ProjectOptions(ctx, project), func(ctx context.Context) error {
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
		_, _ = fmt.Fprintln(s.stdout(), "end of 'compose up' output, interactive run is not supported in dry-run mode")
		return err
	}

	var eg multierror.Group

	// if we get a second signal during shutdown, we kill the services
	// immediately, so the channel needs to have sufficient capacity or
	// we might miss a signal while setting up the second channel read
	// (this is also why signal.Notify is used vs signal.NotifyContext)
	signalChan := make(chan os.Signal, 2)
	defer close(signalChan)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signalChan)
	var isTerminated atomic.Bool

	var (
		logConsumer    = options.Start.Attach
		navigationMenu *formatter.LogKeyboard
		kEvents        <-chan keyboard.KeyEvent
	)
	if options.Start.NavigationMenu {
		kEvents, err = keyboard.GetKeys(100)
		if err != nil {
			logrus.Warnf("could not start menu, an error occurred while starting: %v", err)
			options.Start.NavigationMenu = false
		} else {
			defer keyboard.Close() //nolint:errcheck
			isDockerDesktopActive := s.isDesktopIntegrationActive()
			tracing.KeyboardMetrics(ctx, options.Start.NavigationMenu, isDockerDesktopActive)
			navigationMenu = formatter.NewKeyboardManager(isDockerDesktopActive, signalChan)
			logConsumer = navigationMenu.Decorate(logConsumer)
		}
	}

	tui := formatter.NewStopping(logConsumer)
	defer tui.Close()
	logConsumer = tui

	watcher, err := NewWatcher(project, options, s.watch, logConsumer)
	if err != nil && options.Start.Watch {
		return err
	}

	if navigationMenu != nil && watcher != nil {
		navigationMenu.EnableWatch(options.Start.Watch, watcher)
	}

	printer := newLogPrinter(logConsumer)

	doneCh := make(chan bool)
	eg.Go(func() error {
		first := true
		gracefulTeardown := func() {
			tui.ApplicationTermination()
			eg.Go(func() error {
				return progress.RunWithLog(context.WithoutCancel(ctx), func(ctx context.Context) error {
					return s.stop(ctx, project.Name, api.StopOptions{
						Services: options.Create.Services,
						Project:  project,
					}, printer.HandleEvent)
				}, s.stdinfo(), logConsumer)
			})
			isTerminated.Store(true)
			first = false
		}

		for {
			select {
			case <-doneCh:
				if watcher != nil {
					return watcher.Stop()
				}
				return nil
			case <-ctx.Done():
				if first {
					gracefulTeardown()
				}
			case <-signalChan:
				if first {
					keyboard.Close() //nolint:errcheck
					gracefulTeardown()
					break
				}
				eg.Go(func() error {
					err := s.kill(context.WithoutCancel(ctx), project.Name, api.KillOptions{
						Services: options.Create.Services,
						Project:  project,
						All:      true,
					})
					// Ignore errors indicating that some of the containers were already stopped or removed.
					if cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) {
						return nil
					}

					return err
				})
				return nil
			case event := <-kEvents:
				navigationMenu.HandleKeyEvents(ctx, event, project, options)
			}
		}
	})

	if options.Start.Watch && watcher != nil {
		err = watcher.Start(ctx)
		if err != nil {
			return err
		}
	}

	monitor := newMonitor(s.apiClient(), project)
	monitor.withListener(printer.HandleEvent)

	var exitCode int
	if options.Start.OnExit != api.CascadeIgnore {
		once := true
		// detect first container to exit to trigger application shutdown
		monitor.withListener(func(event api.ContainerEvent) {
			if once && event.Type == api.ContainerEventExited {
				exitCode = event.ExitCode
				printer.Stop()
				_, _ = fmt.Fprintln(s.stdinfo(), "Aborting on container exit...")
				eg.Go(func() error {
					return progress.RunWithLog(context.WithoutCancel(ctx), func(ctx context.Context) error {
						return s.stop(ctx, project.Name, api.StopOptions{
							Services: options.Create.Services,
							Project:  project,
						}, printer.HandleEvent)
					}, s.stdinfo(), logConsumer)
				})
				once = false
			}
		})
	}

	if options.Start.ExitCodeFrom != "" {
		once := true
		// capture exit code from first container to exit with selected service
		monitor.withListener(func(event api.ContainerEvent) {
			if once && event.Type == api.ContainerEventExited && event.Service == options.Start.ExitCodeFrom {
				exitCode = event.ExitCode
				once = false
			}
		})
	}

	monitor.withListener(func(event api.ContainerEvent) {
		mustAttach := false
		switch event.Type {
		case api.ContainerEventCreated:
			// A container has been added to the application (scale)
			mustAttach = true
		case api.ContainerEventStarted:
			// A container is restarting - need to re-attach
			mustAttach = event.Restarting
		}
		if mustAttach {
			eg.Go(func() error {
				// FIXME as container already started, we might miss the very first logs
				return s.doAttachContainer(ctx, event.Service, event.ID, event.Source, printer.HandleEvent)
			})
		}
	})

	eg.Go(func() error {
		err := monitor.Start(ctx)
		fmt.Println("monitor complete")
		// Signal for the signal-handler goroutines to stop
		close(doneCh)
		printer.Stop()
		return err
	})

	// We use the parent context without cancellation as we manage sigterm to stop the stack
	err = s.start(context.WithoutCancel(ctx), project.Name, options.Start, printer.HandleEvent)
	if err != nil && !isTerminated.Load() { // Ignore error if the process is terminated
		return err
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
