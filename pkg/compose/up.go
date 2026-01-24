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
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli"
	"github.com/eiannone/keyboard"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/cmd/formatter"
	"github.com/docker/compose/v5/internal/tracing"
	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Up(ctx context.Context, project *types.Project, options api.UpOptions) error { //nolint:gocyclo
	err := Run(ctx, tracing.SpanWrapFunc("project/up", tracing.ProjectOptions(ctx, project), func(ctx context.Context) error {
		err := s.create(ctx, project, options.Create)
		if err != nil {
			return err
		}
		if options.Start.Attach == nil {
			return s.start(ctx, project.Name, options.Start, nil)
		}
		return nil
	}), "up", s.events)
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

	// if we get a second signal during shutdown, we kill the services
	// immediately, so the channel needs to have sufficient capacity or
	// we might miss a signal while setting up the second channel read
	// (this is also why signal.Notify is used vs signal.NotifyContext)
	signalChan := make(chan os.Signal, 2)
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
			isDockerDesktopActive, err := s.isDesktopIntegrationActive(ctx)
			if err != nil {
				return err
			}
			tracing.KeyboardMetrics(ctx, options.Start.NavigationMenu, isDockerDesktopActive)
			navigationMenu = formatter.NewKeyboardManager(isDockerDesktopActive, signalChan)
			logConsumer = navigationMenu.Decorate(logConsumer)
		}
	}

	watcher, err := NewWatcher(project, options, s.watch, logConsumer)
	if err != nil && options.Start.Watch {
		return err
	}

	if navigationMenu != nil && watcher != nil {
		navigationMenu.EnableWatch(options.Start.Watch, watcher)
	}

	// Detect if the user requested quiet mode
	// logConsumer is nil when --progress quiet or --no-log-prefix is used
	quiet := logConsumer == nil

	var printer logPrinter // <--- sem o *

	if quiet {
		// Create a "silent" printer that ignores normal logs
		// Only critical events like container errors or exit code != 0 will be printed
		printer = newLogPrinter(nil) // mantém simples pra não dar erro de tipo
	} else {
		// Normal printer that writes all logs to the terminal
		printer = newLogPrinter(logConsumer)
	}

	// global context to handle canceling goroutines
	globalCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if navigationMenu != nil {
		navigationMenu.EnableDetach(cancel)
	}

	var (
		eg   errgroup.Group
		mu   sync.Mutex
		errs []error
	)

	appendErr := func(err error) {
		if err != nil {
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		}
	}

	eg.Go(func() error {
		first := true
		gracefulTeardown := func() {
			first = false
			s.events.On(newEvent(api.ResourceCompose, api.Working, api.StatusStopping, "Gracefully Stopping... press Ctrl+C again to force"))
			eg.Go(func() error {
				err = s.stop(context.WithoutCancel(globalCtx), project.Name, api.StopOptions{
					Services: options.Create.Services,
					Project:  project,
				}, printer.HandleEvent)
				appendErr(err)
				return nil
			})
			isTerminated.Store(true)
		}

		for {
			select {
			case <-globalCtx.Done():
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
					_ = keyboard.Close()
					gracefulTeardown()
					break
				}
				eg.Go(func() error {
					err := s.kill(context.WithoutCancel(globalCtx), project.Name, api.KillOptions{
						Services: options.Create.Services,
						Project:  project,
						All:      true,
					})
					// Ignore errors indicating that some of the containers were already stopped or removed.
					if errdefs.IsNotFound(err) || errdefs.IsConflict(err) || errors.Is(err, api.ErrNoResources) {
						return nil
					}

					appendErr(err)
					return nil
				})
				return nil
			case event := <-kEvents:
				navigationMenu.HandleKeyEvents(globalCtx, event, project, options)
			}
		}
	})

	if options.Start.Watch && watcher != nil {
		if err := watcher.Start(globalCtx); err != nil {
			// cancel the global context to terminate background goroutines
			cancel()
			_ = eg.Wait()
			return err
		}
	}

	monitor := newMonitor(s.apiClient(), project.Name)
	if len(options.Start.Services) > 0 {
		monitor.withServices(options.Start.Services)
	} else {
		// Start.AttachTo have been already curated with only the services to monitor
		monitor.withServices(options.Start.AttachTo)
	}
	monitor.withListener(printer.HandleEvent)

	var exitCode int
	if options.Start.OnExit != api.CascadeIgnore {
		once := true
		// detect first container to exit to trigger application shutdown
		monitor.withListener(func(event api.ContainerEvent) {
			if once && event.Type == api.ContainerEventExited {
				if options.Start.OnExit == api.CascadeFail && event.ExitCode == 0 {
					return
				}
				once = false
				exitCode = event.ExitCode
				s.events.On(newEvent(api.ResourceCompose, api.Working, api.StatusStopping, "Aborting on container exit..."))
				eg.Go(func() error {
					err = s.stop(context.WithoutCancel(globalCtx), project.Name, api.StopOptions{
						Services: options.Create.Services,
						Project:  project,
					}, printer.HandleEvent)
					appendErr(err)
					return nil
				})
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

	containers, err := s.attach(globalCtx, project, printer.HandleEvent, options.Start.AttachTo)
	if err != nil {
		cancel()
		_ = eg.Wait()
		return err
	}
	attached := make([]string, len(containers))
	for i, ctr := range containers {
		attached[i] = ctr.ID
	}

	monitor.withListener(func(event api.ContainerEvent) {
		if event.Type != api.ContainerEventStarted {
			return
		}
		if slices.Contains(attached, event.ID) && !event.Restarting {
			return
		}
		eg.Go(func() error {
			ctr, err := s.apiClient().ContainerInspect(globalCtx, event.ID)
			if err != nil {
				appendErr(err)
				return nil
			}

			err = s.doLogContainer(globalCtx, options.Start.Attach, event.Source, ctr, api.LogOptions{
				Follow: true,
				Since:  ctr.State.StartedAt,
			})
			if errdefs.IsNotImplemented(err) {
				// container may be configured with logging_driver: none
				// as container already started, we might miss the very first logs. But still better than none
				err := s.doAttachContainer(globalCtx, event.Service, event.ID, event.Source, printer.HandleEvent)
				appendErr(err)
				return nil
			}
			appendErr(err)
			return nil
		})
	})

	eg.Go(func() error {
		err := monitor.Start(globalCtx)
		// cancel the global context to terminate signal-handler goroutines
		cancel()
		appendErr(err)
		return nil
	})

	// We use the parent context without cancellation as we manage sigterm to stop the stack
	err = s.start(context.WithoutCancel(ctx), project.Name, options.Start, printer.HandleEvent)
	if err != nil && !isTerminated.Load() { // Ignore error if the process is terminated
		cancel()
		_ = eg.Wait()
		return err
	}

	_ = eg.Wait()
	err = errors.Join(errs...)
	if exitCode != 0 {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		return cli.StatusError{StatusCode: exitCode, Status: errMsg}
	}
	return err
}
