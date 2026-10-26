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
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/docker/compose/v5/internal/desktop"
	"github.com/docker/compose/v5/internal/tracing"
	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Up(ctx context.Context, project *types.Project, options api.UpOptions) error {
	if err := options.Validate(); err != nil {
		return err
	}
	var created *createdUp
	err := Run(ctx, tracing.SpanWrapFunc("project/up", tracing.ProjectOptions(ctx, project), func(ctx context.Context) error {
		if options.Start.Attach == nil {
			return s.upDetached(ctx, project, options)
		}
		var err error
		created, err = s.upCreatePhase(ctx, project, options)
		return err
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
	return s.runInteractiveUp(ctx, created, options)
}

// createdUp is what an interactive up carries from its Create phase to its
// Start phase: the canonical project the plan was built for, and the plan run
// whose Start phase is still to dispatch. Both phases share the one daemon
// snapshot and the one plan the Create phase was prepared from.
type createdUp struct {
	project *types.Project
	run     *planRun
}

// upCreatePhase prepares interactive up's single Create+Start plan and runs
// its Create phase only, under the "up" progress scope the caller wraps it in.
// The Start phase is deliberately left out of that scope: it runs once
// runInteractiveUp has set up the session streaming logs to the terminal, and
// the progress display's refresh would repaint over them.
func (s *composeService) upCreatePhase(ctx context.Context, project *types.Project, options api.UpOptions) (*createdUp, error) {
	project, exec, plan, err := s.prepareUp(ctx, project, options)
	if err != nil {
		return nil, err
	}

	run, err := exec.begin(plan)
	if err != nil {
		return nil, err
	}
	if err := run.runCreate(ctx); err != nil {
		return nil, err
	}
	return &createdUp{project: project, run: run}, nil
}

// prepareUp prepares the single Create+Start plan of an up, detached or
// interactive, and the executor that runs it. It returns the plan's own
// project, the one carrying what preparation resolved (see useAPISocket).
func (s *composeService) prepareUp(ctx context.Context, project *types.Project, options api.UpOptions) (*types.Project, *planExecutor, *Plan, error) {
	project, observed, plan, err := s.preparePlan(ctx, project, options.Create, ScopeCreateStart)
	if err != nil {
		return nil, nil, nil, err
	}

	// Must run against the pre-execution snapshot: observed only labels a
	// container Running if it already was one before this plan touched
	// anything, exactly what "the plan won't touch it" is supposed to mean.
	emitRunningEvents(project, observed, plan, s.events)

	// The plan's own OpWaitCondition nodes (service_healthy /
	// service_completed_successfully dependency waits) block with no timeout
	// of their own -- unlike start()'s per-dependency waitDependencies call,
	// which already threads WaitTimeout through every wait unconditionally
	// (InDependencyOrder passes it to startService regardless of
	// options.Wait -- --wait-timeout alone, with no --wait, is a legal CLI
	// combination nothing rejects). exec.waitTimeout bounds exactly those
	// nodes (see executeNode), each with its OWN fresh window starting when
	// that wait begins -- not a single shared budget counted down from
	// before the plan even started. Sharing one absolute deadline across
	// create/start/hook work and every wait would let slow, unrelated work
	// alone exhaust it, failing an otherwise healthy deployment as "not
	// healthy" with no wait ever actually timing out -- start()'s real,
	// per-wait-independent behavior never had that failure mode.
	exec := s.newPlanExecutor(project, observed)
	exec.waitTimeout = options.Start.WaitTimeout
	return project, exec, plan, nil
}

// upDetached runs detached `up`'s Create and Start phases as a single plan:
// the semantic switchover from create() + start() -- two separate daemon
// snapshots, the second blind to what the first just did -- to one
// preparePlan/executePlan pass covering both phases (epic #14081, lot 2).
// Interactive up prepares the same plan but drives its two phases itself,
// with the attach/printer/monitor setup in between (see upCreatePhase).
func (s *composeService) upDetached(ctx context.Context, project *types.Project, options api.UpOptions) error {
	project, exec, plan, err := s.prepareUp(ctx, project, options)
	if err != nil {
		return err
	}

	if err := exec.run(ctx, plan); err != nil {
		return translateWaitTimeout(err)
	}

	if !options.Start.Wait {
		return nil
	}

	// The plan may have created or recreated containers: this verification
	// needs their current IDs, which the pre-execution observed snapshot
	// above doesn't have for a container that didn't exist yet at
	// observation time. The listing runs on the caller's context, before the
	// --wait-timeout window opens, so daemon-listing latency never eats into
	// the budget the readiness polling is owed (any deadline it hits is the
	// caller's own, unrelated to --wait-timeout, and is returned as such).
	containers, err := s.getContainers(ctx, project.Name, oneOffExclude, true)
	if err != nil {
		return err
	}

	// The check then opens its own fresh WaitTimeout window, same as every
	// OpWaitCondition node above and start()'s own final --wait check did --
	// not whatever's left of a shared budget, which unrelated plan work could
	// already have exhausted with no wait ever at risk.
	//
	// origCtx is kept so the DeadlineExceeded check below can tell this fresh
	// window expiring (origCtx still fine) apart from origCtx's own,
	// independent deadline propagating through the derived one (origCtx
	// already done): a parent deadline firing first makes the derived
	// context's Err() report DeadlineExceeded too, inherited from the
	// parent, even though this window's own timer never fired.
	origCtx := ctx
	if options.Start.WaitTimeout > 0 {
		withTimeout, cancel := context.WithTimeout(ctx, options.Start.WaitTimeout)
		defer cancel()
		ctx = withTimeout
	}

	depends := types.DependsOnConfig{}
	for _, svc := range project.Services {
		depends[svc.Name] = types.ServiceDependency{
			Condition: getDependencyCondition(svc, project),
			Required:  true,
		}
	}
	if err := s.waitDependencies(ctx, project, project.Name, depends, containers, 0); err != nil {
		if options.Start.WaitTimeout > 0 && origCtx.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("application not healthy after %s", options.Start.WaitTimeout)
		}
		return err
	}
	return nil
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
	menu       NavigationMenu
	globalCtx  context.Context
	cancel     context.CancelFunc
	// logOpenLimiter bounds concurrent log-attach opens from
	// followStartedContainers, the same footgun --parallel closes for
	// `compose logs --follow` (see doLogContainer).
	logOpenLimiter *semaphore.Weighted

	signalChan   chan os.Signal
	isTerminated atomic.Bool
	eg           errgroup.Group
	mu           sync.Mutex
	errs         []error
	exitCode     int
}

// runInteractiveUp sets up the attach/printer/monitor session around the
// Start phase of created's plan, runs it, then follows the application until
// it terminates. The Create phase already ran: the session's own signal
// handling only starts after it, so an interruption of the Create phase is the
// caller's, through ctx. An interruption met while the session attaches or
// reads the daemon's time starts nothing; one arriving later does not stop the
// Start phase, which runs on a context that outlives it.
func (s *composeService) runInteractiveUp(ctx context.Context, created *createdUp, options api.UpOptions) error {
	// the plan's own project, not the caller's: the one the Start phase runs
	// on, so the session, the watcher and the teardown follow the same model
	project := created.project

	// if we get a second signal during shutdown, we kill the services
	// immediately, so the channel needs to have sufficient capacity or
	// we might miss a signal while setting up the second channel read
	// (this is also why signal.Notify is used vs signal.NotifyContext)
	signalChan := make(chan os.Signal, 2)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signalChan)

	logConsumer := options.Start.Attach
	navigationMenu, err := s.setupNavigationMenu(ctx, &options, signalChan)
	if err != nil {
		return err
	}
	if navigationMenu != nil {
		defer navigationMenu.Close() //nolint:errcheck
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
		logOpenLimiter: newOptionalLimiter(s.maxConcurrency),
		signalChan:     signalChan,
	}

	u.eg.Go(func() error {
		return u.runEventLoop(ctx)
	})
	if navigationMenu != nil {
		u.eg.Go(func() error {
			navigationMenu.Run(globalCtx, project, options)
			return nil
		})
	}

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

	// The monitor replays events from the daemon's time. Read it now, before
	// the Start phase: the point precedes the first start, and a daemon that
	// answers with a time that can't be used fails the command without any
	// container started. A daemon that can't be asked doesn't (see
	// readDaemonTime): the monitor then follows live events.
	if err := monitor.readDaemonTime(globalCtx); err != nil {
		cancel()
		_ = u.eg.Wait()
		return err
	}

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
		case <-time.After(u.drainTimeout()):
		case <-globalCtx.Done():
		}
		// cancel the global context to terminate signal-handler goroutines
		cancel()
		u.appendErr(err)
		return nil
	})

	// We use the parent context without cancellation as we manage sigterm to stop the stack
	err = created.run.runStart(context.WithoutCancel(ctx), u.printer.HandleEvent)
	err = translateWaitTimeout(err)
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
// It returns a nil menu when the menu is disabled, when no menu implementation
// was configured (WithNavigationMenu), or when the keyboard can't be grabbed —
// then disabling the option.
func (s *composeService) setupNavigationMenu(ctx context.Context, options *api.UpOptions, signalChan chan os.Signal) (NavigationMenu, error) {
	if !options.Start.NavigationMenu {
		return nil, nil
	}
	if s.navigationMenu == nil {
		logrus.Warn("no navigation menu implementation configured (see WithNavigationMenu), disabling the option")
		options.Start.NavigationMenu = false
		return nil, nil
	}
	isDockerDesktopActive, err := s.isDesktopIntegrationActive(ctx)
	if err != nil {
		return nil, err
	}
	isLogsViewEnabled := s.isDesktopFeatureActive(ctx, desktop.FeatureLogsTab)
	menu := s.navigationMenu(isDockerDesktopActive, isLogsViewEnabled, signalChan)
	if menu == nil {
		options.Start.NavigationMenu = false
		return nil, nil
	}
	if err := menu.Open(); err != nil {
		logrus.Warnf("could not start menu, an error occurred while starting: %v", err)
		options.Start.NavigationMenu = false
		return nil, nil
	}
	tracing.KeyboardMetrics(ctx, options.Start.NavigationMenu, isDockerDesktopActive, isLogsViewEnabled)
	return menu, nil
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

// runEventLoop reacts to cancellation and SIGINT/SIGTERM until
// the application terminates: a first interruption triggers a graceful stop,
// a second one kills the services.
func (u *upSession) runEventLoop(ctx context.Context) error {
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
				if u.menu != nil {
					_ = u.menu.Close()
				}
				gracefulTeardown()
				break
			}
			u.killApplication()
			return nil
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
			Timeout:  u.options.Create.Timeout,
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
			Timeout:  u.options.Create.Timeout,
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

// defaultLogStreamDrainTimeout is the default bound on the shutdown drain of the
// streams opened by followStartedContainers: EOF is guaranteed once the containers exited, the bound only protects against a
// wedged daemon holding the connection open. It is a composeService field so
// tests can shrink it without touching package state.
const defaultLogStreamDrainTimeout = 5 * time.Second

// drainTimeout returns the configured drain bound, falling back to the default
// for a composeService not built by NewComposeService.
func (s *composeService) drainTimeout() time.Duration {
	if s.logStreamDrainTimeout > 0 {
		return s.logStreamDrainTimeout
	}
	return defaultLogStreamDrainTimeout
}

// followStartedContainers streams logs of containers (re)started after `up`,
// so they are followed like the initially attached ones.
func (u *upSession) followStartedContainers(attached []string) api.ContainerEventListener {
	runEnds := newRunEndTracker()
	cursors := newLogCursors()
	return func(event api.ContainerEvent) {
		runEnds.Observe(event)
		if !shouldFollowStartEvent(event, attached, u.options.Start.AttachTo) {
			return
		}
		// Captured synchronously — see followStartedContainersLogs: read any
		// later, a fast run's own exit could already be recorded and the log
		// window would drop the whole run.
		since := runEnds.Since(event.ID)
		// Queued here, in event order, for the same reason.
		turn := cursors.enter(event.ID)
		// counted before the goroutine starts so the shutdown drain can never
		// miss a stream dispatched but not yet running
		u.logStreams.Add(1)
		u.eg.Go(func() error {
			defer u.logStreams.Done()
			defer turn.end()
			u.appendErr(u.streamContainerLogs(turn, event, since))
			return nil
		})
	}
}

func (u *upSession) streamContainerLogs(turn *logTurn, event api.ContainerEvent, since string) error {
	if err := turn.wait(u.globalCtx); err != nil {
		return err
	}
	ctr, err := u.inspectWithSlot(u.globalCtx, u.logOpenLimiter, event.ID)
	if err != nil {
		return err
	}
	if since == "" {
		since = logsSinceLastRun(ctr)
	}

	err = u.doLogContainer(u.globalCtx, u.logOpenLimiter, u.options.Start.Attach, event.Source, ctr, api.LogOptions{
		Follow: true,
		Since:  since,
	}, turn.cursor)
	if errdefs.IsNotImplemented(err) {
		// container may be configured with logging_driver: none
		// as container already started, we might miss the very first logs. But still better than none
		turn.end() // nothing was relayed, and the attach lasts as long as the run
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
