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
	"time"

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
	project *types.Project
	options api.UpOptions
	printer logPrinter
	// logStreams counts the in-flight re-attach log streams so shutdown can
	// drain them before tearing the context down — see the monitor wrapper.
	logStreams sync.WaitGroup
	watcher    *Watcher
	menu       *formatter.LogKeyboard
	globalCtx  context.Context
	cancel     context.CancelFunc

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

	// Termination -- on-exit cascade or a graceful Ctrl+C/SIGTERM teardown --
	// stops the application via a one-shot listing (see stopApplication):
	// registered unconditionally, regardless of the on-exit policy, since a
	// graceful teardown always runs it and a container whose start was
	// already in flight when that listing happened comes up after it.
	monitor.withListener(u.stopLateStarters())
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
		// The monitor returning means every watched container is gone for
		// good — an events-channel fact. The last run's log lines may still
		// be in flight on their own connections, and canceling now would
		// drop them: the exit notice would outrun the output that preceded
		// it. The containers having exited, every follow stream terminates
		// on its own at EOF — give them a bounded window to drain before
		// the context comes down (immediately skipped when the context is
		// already canceled, e.g. Ctrl-C).
		drained := make(chan struct{})
		go func() {
			u.logStreams.Wait()
			close(drained)
		}()
		select {
		case <-drained:
		case <-time.After(logStreamDrainTimeout):
		case <-globalCtx.Done():
		}
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

// appendErr records err for the final report, unless it is nothing more than
// fallout from our own shutdown: once u.globalCtx is canceled (monitor
// detecting termination, SIGINT/SIGTERM, or an earlier setup failure), the
// in-flight goroutines it carries (log/attach streaming in particular) get a
// context.Canceled error that reports no real failure and must not turn a
// clean exit into a non-zero one (#13985).
func (u *upSession) appendErr(err error) {
	if err == nil {
		return
	}
	if errors.Is(err, context.Canceled) && u.globalCtx.Err() != nil {
		logrus.Debugf("ignoring canceled error after shutdown: %v", err)
		return
	}
	u.mu.Lock()
	u.errs = append(u.errs, err)
	u.mu.Unlock()
}

// runEventLoop reacts to cancellation, SIGINT/SIGTERM and keyboard input until
// the application terminates: a first interruption triggers a graceful stop,
// a second one kills the services.
func (u *upSession) runEventLoop(ctx context.Context, kEvents <-chan keyboard.KeyEvent) error {
	first := true
	gracefulTeardown := func() {
		first = false
		u.events.On(newEvent(api.ResourceCompose, api.Working, api.StatusStopping, "Gracefully Stopping... press Ctrl+C again to force"))
		// set before stopApplication's listing so stopLateStarters is armed
		// no later than the sweep it must catch stragglers for.
		u.isTerminated.Store(true)
		u.stopApplication()
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
		if !once {
			return
		}
		if event.Type != api.ContainerEventExited {
			return
		}
		if u.options.Start.OnExit == api.CascadeFail && event.ExitCode == 0 {
			return
		}
		once = false
		u.exitCode = event.ExitCode
		u.events.On(newEvent(api.ResourceCompose, api.Working, api.StatusStopping, "Aborting on container exit..."))
		// set before stopApplication's listing so stopLateStarters is armed
		// no later than the sweep it must catch stragglers for.
		u.isTerminated.Store(true)
		u.stopApplication()
	}
}

// stopLateStarters stops any service that starts after termination has begun
// — the on-exit cascade above or a graceful Ctrl+C/SIGTERM teardown
// (runEventLoop) — both of which stop the application via stopApplication's
// one-shot listing. Either start phase (the initial one, or one still
// climbing the dependency graph on its deliberately uncancelable context)
// can race that listing: a container whose start was already in flight comes
// up after the sweep and would otherwise keep the session alive until its
// natural end. The events stream reveals such late starters — stop each one
// as it appears, for as long as termination is underway.
func (u *upSession) stopLateStarters() api.ContainerEventListener {
	return func(event api.ContainerEvent) {
		if !isLateStarter(event, u.isTerminated.Load()) {
			return
		}
		u.stopLateStarter(event.Service)
	}
}

// isLateStarter reports whether event is a container starting after
// termination has begun — the on-exit cascade or a graceful Ctrl+C/SIGTERM
// teardown, either of which sets terminated true before its one-shot stop
// listing — and so must be caught and stopped: see stopLateStarters.
func isLateStarter(event api.ContainerEvent, terminated bool) bool {
	return terminated && event.Type == api.ContainerEventStarted
}

// stopLateStarter stops one service started after termination swept the
// application — see stopLateStarters.
func (u *upSession) stopLateStarter(service string) {
	u.eg.Go(func() error {
		err := u.stop(context.WithoutCancel(u.globalCtx), u.project.Name, api.StopOptions{
			Services: []string{service},
			Project:  u.project,
		}, u.printer.HandleEvent)
		u.appendErr(err)
		return nil
	})
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
//
// logStreamDrainTimeout bounds the shutdown drain of these streams: EOF is
// guaranteed once the containers exited, the bound only protects against a
// wedged daemon holding the connection open. A variable so tests can shrink
// it.
var logStreamDrainTimeout = 5 * time.Second

func (u *upSession) followStartedContainers(attached []string) api.ContainerEventListener {
	runEnds := newRunEndTracker()
	return func(event api.ContainerEvent) {
		runEnds.Observe(event)
		if !shouldFollowStartEvent(event, attached, u.options.Start.AttachTo) {
			return
		}
		// Captured synchronously — see followStartedContainersLogs: read any
		// later, a fast run's own exit could already be recorded and the log
		// window would drop the whole run.
		since := runEnds.Since(event.ID)
		// counted before the goroutine starts so the shutdown drain can never
		// miss a stream dispatched but not yet running
		u.logStreams.Add(1)
		u.eg.Go(func() error {
			defer u.logStreams.Done()
			u.appendErr(u.streamContainerLogs(event, since))
			return nil
		})
	}
}

func (u *upSession) streamContainerLogs(event api.ContainerEvent, since string) error {
	res, err := u.apiClient().ContainerInspect(u.globalCtx, event.ID, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	if since == "" {
		since = logsSinceLastRun(res.Container)
	}

	err = u.doLogContainer(u.globalCtx, u.options.Start.Attach, event.Source, res.Container, api.LogOptions{
		Follow: true,
		Since:  since,
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
