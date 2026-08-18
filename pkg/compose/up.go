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
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/cmd/formatter"
	"github.com/docker/compose/v5/internal/desktop"
	"github.com/docker/compose/v5/internal/tracing"
	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Up(ctx context.Context, project *types.Project, options api.UpOptions) error {
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
	return s.runInteractiveUp(ctx, project, options)
}

// upSession carries the state shared between the goroutines driving an
// interactive `compose up` once services are created: the errgroup running
// them, the collected errors, and the application exit status.
type upSession struct {
	*composeService
	project   *types.Project
	options   api.UpOptions
	printer   logPrinter
	watcher   *Watcher
	menu      *formatter.LogKeyboard
	globalCtx context.Context
	cancel    context.CancelFunc

	signalChan   chan os.Signal
	isTerminated atomic.Bool
	eg           errgroup.Group
	mu           sync.Mutex
	errs         []error
	exitCode     int
}

func (s *composeService) runInteractiveUp(ctx context.Context, project *types.Project, options api.UpOptions) error {
	// if we get a second signal during shutdown, we kill the services
	// immediately, so the channel needs to have sufficient capacity or
	// we might miss a signal while setting up the second channel read
	// (this is also why signal.Notify is used vs signal.NotifyContext)
	signalChan := make(chan os.Signal, 2)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signalChan)

	logConsumer := options.Start.Attach
	navigationMenu, kEvents, err := s.setupNavigationMenu(ctx, &options, signalChan)
	if err != nil {
		return err
	}
	if navigationMenu != nil {
		defer keyboard.Close() //nolint:errcheck
		logConsumer = navigationMenu.Decorate(logConsumer)
	}

	watcher, err := NewWatcher(project, options, s.watch, logConsumer)
	if err != nil && options.Start.Watch {
		return err
	}

	if navigationMenu != nil && watcher != nil {
		navigationMenu.EnableWatch(options.Start.Watch, watcher)
	}

	// global context to handle canceling goroutines
	globalCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if navigationMenu != nil {
		navigationMenu.EnableDetach(cancel)
	}

	u := &upSession{
		composeService: s,
		project:        project,
		options:        options,
		printer:        newLogPrinter(logConsumer),
		watcher:        watcher,
		menu:           navigationMenu,
		globalCtx:      globalCtx,
		cancel:         cancel,
		signalChan:     signalChan,
	}

	u.eg.Go(func() error {
		return u.runEventLoop(ctx, kEvents)
	})

	if options.Start.Watch && watcher != nil {
		if err := watcher.Start(globalCtx); err != nil {
			// cancel the global context to terminate background goroutines
			cancel()
			_ = u.eg.Wait()
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
	monitor.withListener(u.printer.HandleEvent)

	if options.Start.OnExit != api.CascadeIgnore {
		monitor.withListener(u.stopOnFirstExit())
	}
	if options.Start.ExitCodeFrom != "" {
		monitor.withListener(u.captureExitCodeFrom())
	}

	containers, err := s.attach(globalCtx, project, u.printer.HandleEvent, options.Start.AttachTo)
	if err != nil {
		cancel()
		_ = u.eg.Wait()
		return err
	}
	attached := make([]string, len(containers))
	for i, ctr := range containers {
		attached[i] = ctr.ID
	}
	monitor.withListener(u.followStartedContainers(attached))

	u.eg.Go(func() error {
		err := monitor.Start(globalCtx)
		// cancel the global context to terminate signal-handler goroutines
		cancel()
		u.appendErr(err)
		return nil
	})

	// We use the parent context without cancellation as we manage sigterm to stop the stack
	err = s.start(context.WithoutCancel(ctx), project.Name, options.Start, u.printer.HandleEvent)
	if err != nil && !u.isTerminated.Load() { // Ignore error if the process is terminated
		cancel()
		_ = u.eg.Wait()
		return err
	}

	_ = u.eg.Wait()
	err = errors.Join(u.errs...)
	if u.exitCode != 0 {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		return cli.StatusError{StatusCode: u.exitCode, Status: errMsg}
	}
	return err
}

// setupNavigationMenu initializes the interactive keyboard menu when enabled.
// It returns a nil menu when the menu is disabled, or when the keyboard can't
// be grabbed — then disabling the option.
func (s *composeService) setupNavigationMenu(ctx context.Context, options *api.UpOptions, signalChan chan os.Signal) (*formatter.LogKeyboard, <-chan keyboard.KeyEvent, error) {
	if !options.Start.NavigationMenu {
		return nil, nil, nil
	}
	kEvents, err := keyboard.GetKeys(100)
	if err != nil {
		logrus.Warnf("could not start menu, an error occurred while starting: %v", err)
		options.Start.NavigationMenu = false
		return nil, nil, nil
	}
	isDockerDesktopActive, err := s.isDesktopIntegrationActive(ctx)
	if err != nil {
		_ = keyboard.Close()
		return nil, nil, err
	}
	isLogsViewEnabled := s.isDesktopFeatureActive(ctx, desktop.FeatureLogsTab)
	tracing.KeyboardMetrics(ctx, options.Start.NavigationMenu, isDockerDesktopActive, isLogsViewEnabled)
	return formatter.NewKeyboardManager(isDockerDesktopActive, isLogsViewEnabled, signalChan), kEvents, nil
}

func (u *upSession) appendErr(err error) {
	if err != nil {
		u.mu.Lock()
		u.errs = append(u.errs, err)
		u.mu.Unlock()
	}
}

// runEventLoop reacts to cancellation, SIGINT/SIGTERM and keyboard input until
// the application terminates: a first interruption triggers a graceful stop,
// a second one kills the services.
func (u *upSession) runEventLoop(ctx context.Context, kEvents <-chan keyboard.KeyEvent) error {
	first := true
	gracefulTeardown := func() {
		first = false
		u.events.On(newEvent(api.ResourceCompose, api.Working, api.StatusStopping, "Gracefully Stopping... press Ctrl+C again to force"))
		u.stopApplication()
		u.isTerminated.Store(true)
	}

	for {
		select {
		case <-u.globalCtx.Done():
			if u.watcher != nil {
				return u.watcher.Stop()
			}
			return nil
		case <-ctx.Done():
			if first {
				gracefulTeardown()
			}
		case <-u.signalChan:
			if first {
				_ = keyboard.Close()
				gracefulTeardown()
				break
			}
			u.killApplication()
			return nil
		case event := <-kEvents:
			u.menu.HandleKeyEvents(u.globalCtx, event, u.project, u.options)
		}
	}
}

// stopApplication requests a graceful stop of the application services in
// background; the error is collected for the final report.
func (u *upSession) stopApplication() {
	u.eg.Go(func() error {
		err := u.stop(context.WithoutCancel(u.globalCtx), u.project.Name, api.StopOptions{
			Services: u.options.Create.Services,
			Project:  u.project,
		}, u.printer.HandleEvent)
		u.appendErr(err)
		return nil
	})
}

// killApplication kills the application services in background; the error is
// collected for the final report.
func (u *upSession) killApplication() {
	u.eg.Go(func() error {
		err := u.kill(context.WithoutCancel(u.globalCtx), u.project.Name, api.KillOptions{
			Services: u.options.Create.Services,
			Project:  u.project,
			All:      true,
		})
		// Ignore errors indicating that some of the containers were already stopped or removed.
		if errdefs.IsNotFound(err) || errdefs.IsConflict(err) || errors.Is(err, api.ErrNoResources) {
			return nil
		}

		u.appendErr(err)
		return nil
	})
}

// stopOnFirstExit detects the first container to exit — per the on-exit
// cascade policy — to trigger application shutdown and record the
// application exit code.
func (u *upSession) stopOnFirstExit() api.ContainerEventListener {
	once := true
	return func(event api.ContainerEvent) {
		if !once || event.Type != api.ContainerEventExited {
			return
		}
		if u.options.Start.OnExit == api.CascadeFail && event.ExitCode == 0 {
			return
		}
		once = false
		u.exitCode = event.ExitCode
		u.events.On(newEvent(api.ResourceCompose, api.Working, api.StatusStopping, "Aborting on container exit..."))
		u.stopApplication()
	}
}

// captureExitCodeFrom captures the exit code of the first container to exit
// for the service selected by --exit-code-from
func (u *upSession) captureExitCodeFrom() api.ContainerEventListener {
	once := true
	return func(event api.ContainerEvent) {
		if once && event.Type == api.ContainerEventExited && event.Service == u.options.Start.ExitCodeFrom {
			u.exitCode = event.ExitCode
			once = false
		}
	}
}

// followStartedContainers streams logs of containers (re)started after `up`,
// so they are followed like the initially attached ones.
func (u *upSession) followStartedContainers(attached []string) api.ContainerEventListener {
	return func(event api.ContainerEvent) {
		if !shouldFollowStartEvent(event, attached, u.options.Start.AttachTo) {
			return
		}
		u.eg.Go(func() error {
			u.appendErr(u.streamContainerLogs(event))
			return nil
		})
	}
}

func (u *upSession) streamContainerLogs(event api.ContainerEvent) error {
	res, err := u.apiClient().ContainerInspect(u.globalCtx, event.ID, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}

	err = u.doLogContainer(u.globalCtx, u.options.Start.Attach, event.Source, res.Container, api.LogOptions{
		Follow: true,
		Since:  res.Container.State.StartedAt,
	})
	if errdefs.IsNotImplemented(err) {
		// container may be configured with logging_driver: none
		// as container already started, we might miss the very first logs. But still better than none
		return u.doAttachContainer(u.globalCtx, event.Service, event.ID, event.Source, u.printer.HandleEvent)
	}
	return err
}

func shouldFollowStartEvent(event api.ContainerEvent, attached []string, attachTo []string) bool {
	if event.Type != api.ContainerEventStarted {
		return false
	}
	if len(attachTo) > 0 && !slices.Contains(attachTo, event.Service) {
		return false
	}
	if slices.Contains(attached, event.ID) && !event.Restarting {
		return false
	}
	return true
}
