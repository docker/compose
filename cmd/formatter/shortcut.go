/*
   Copyright 2024 Docker Compose CLI authors

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

package formatter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"syscall"
	"time"

	"github.com/buger/goterm"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/internal/tracing"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/watch"
	"github.com/eiannone/keyboard"
	"github.com/hashicorp/go-multierror"
	"github.com/skratchdot/open-golang/open"
)

const DISPLAY_ERROR_TIME = 10

type KeyboardError struct {
	err       error
	timeStart time.Time
}

func (ke *KeyboardError) shouldDisplay() bool {
	return ke.err != nil && int(time.Since(ke.timeStart).Seconds()) < DISPLAY_ERROR_TIME
}

func (ke *KeyboardError) printError(height int, info string) {
	if ke.shouldDisplay() {
		errMessage := ke.err.Error()

		MoveCursor(height-1-extraLines(info)-extraLines(errMessage), 0)
		ClearLine()

		fmt.Print(errMessage)
	}
}

func (ke *KeyboardError) addError(prefix string, err error) {
	ke.timeStart = time.Now()

	prefix = ansiColor(CYAN, fmt.Sprintf("%s →", prefix), BOLD)
	errorString := fmt.Sprintf("%s  %s", prefix, err.Error())

	ke.err = errors.New(errorString)
}

func (ke *KeyboardError) error() string {
	return ke.err.Error()
}

type KeyboardWatch struct {
	Watcher  watch.Notify
	Watching bool
	WatchFn  func(ctx context.Context, doneCh chan bool, project *types.Project, services []string, options api.WatchOptions) error
	Ctx      context.Context
	Cancel   context.CancelFunc
}

func (kw *KeyboardWatch) isWatching() bool {
	return kw.Watching
}

func (kw *KeyboardWatch) switchWatching() {
	kw.Watching = !kw.Watching
}

func (kw *KeyboardWatch) newContext(ctx context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	kw.Ctx = ctx
	kw.Cancel = cancel
	return cancel
}

type KEYBOARD_LOG_LEVEL int

const (
	NONE  KEYBOARD_LOG_LEVEL = 0
	INFO  KEYBOARD_LOG_LEVEL = 1
	DEBUG KEYBOARD_LOG_LEVEL = 2
)

type LogKeyboard struct {
	kError                KeyboardError
	Watch                 KeyboardWatch
	IsDockerDesktopActive bool
	IsWatchConfigured     bool
	IsDDComposeUIActive   bool
	logLevel              KEYBOARD_LOG_LEVEL
	signalChannel         chan<- os.Signal
}

var KeyboardManager *LogKeyboard
var eg multierror.Group

func NewKeyboardManager(ctx context.Context, isDockerDesktopActive, isWatchConfigured, isDockerDesktopConfigActive bool,
	sc chan<- os.Signal,
	watchFn func(ctx context.Context,
		doneCh chan bool,
		project *types.Project,
		services []string,
		options api.WatchOptions,
	) error,
) {
	km := LogKeyboard{}
	km.IsDockerDesktopActive = isDockerDesktopActive
	km.IsWatchConfigured = isWatchConfigured
	km.IsDDComposeUIActive = isDockerDesktopConfigActive
	km.logLevel = INFO

	km.Watch.Watching = false
	km.Watch.WatchFn = watchFn

	km.signalChannel = sc

	KeyboardManager = &km
}

func (lk *LogKeyboard) ClearKeyboardInfo() {
	lk.clearNavigationMenu()
}

func (lk *LogKeyboard) PrintKeyboardInfo() {
	if lk.logLevel == INFO {
		lk.printNavigationMenu()
	}
}

// Creates space to print error and menu string
func (lk *LogKeyboard) createBuffer(lines int) {
	if lk.kError.shouldDisplay() {
		extraLines := extraLines(lk.kError.error()) + 1
		lines += extraLines
	}

	// get the string
	infoMessage := lk.navigationMenu()
	// calculate how many lines we need to display the menu info
	// might be needed a line break
	extraLines := extraLines(infoMessage) + 1
	lines += extraLines

	if lines > 0 {
		allocateSpace(lines)
		MoveCursorUp(lines)
	}
}

func (lk *LogKeyboard) printNavigationMenu() {
	offset := 1
	lk.clearNavigationMenu()
	lk.createBuffer(offset)

	if lk.logLevel == INFO {
		height := goterm.Height()
		menu := lk.navigationMenu()

		MoveCursorX(0)
		SaveCursor()

		lk.kError.printError(height, menu)

		MoveCursor(height-extraLines(menu), 0)
		ClearLine()
		fmt.Print(menu)

		MoveCursorX(0)
		RestoreCursor()
	}
}

func (lk *LogKeyboard) navigationMenu() string {
	var openDDInfo string
	if lk.IsDockerDesktopActive {
		openDDInfo = shortcutKeyColor("v") + navColor(" View in Docker Desktop")
	}

	var openDDUI string
	if openDDInfo != "" {
		openDDUI = navColor("   ")
	}
	if lk.IsDDComposeUIActive {
		openDDUI = openDDUI + shortcutKeyColor("o") + navColor(" View Config")
	}
	var watchInfo string
	if openDDInfo != "" || openDDUI != "" {
		watchInfo = navColor("   ")
	}
	var isEnabled = " Enable"
	if lk.Watch.Watching {
		isEnabled = " Disable"
	}
	watchInfo = watchInfo + shortcutKeyColor("w") + navColor(isEnabled+" Watch")
	return openDDInfo + openDDUI + watchInfo
}

func (lk *LogKeyboard) clearNavigationMenu() {
	height := goterm.Height()
	MoveCursorX(0)
	SaveCursor()

	// ClearLine()
	for i := 0; i < height; i++ {
		MoveCursorDown(1)
		ClearLine()
	}
	RestoreCursor()
}

func (lk *LogKeyboard) openDockerDesktop(ctx context.Context, project *types.Project) {
	if !lk.IsDockerDesktopActive {
		return
	}
	eg.Go(tracing.EventWrapFuncForErrGroup(ctx, "menu/gui", tracing.SpanOptions{},
		func(ctx context.Context) error {
			link := fmt.Sprintf("docker-desktop://dashboard/apps/%s", project.Name)
			err := open.Run(link)
			if err != nil {
				err = fmt.Errorf("Could not open Docker Desktop")
				lk.keyboardError("View", err)
			}
			return err
		}),
	)
}

func (lk *LogKeyboard) openDDComposeUI(ctx context.Context, project *types.Project) {
	if !lk.IsDDComposeUIActive {
		return
	}
	eg.Go(tracing.EventWrapFuncForErrGroup(ctx, "menu/gui/composeview", tracing.SpanOptions{},
		func(ctx context.Context) error {
			link := fmt.Sprintf("docker-desktop://dashboard/docker-compose/%s", project.Name)
			err := open.Run(link)
			if err != nil {
				err = fmt.Errorf("Could not open Docker Desktop Compose UI")
				lk.keyboardError("View Config", err)
			}
			return err
		}),
	)
}
func (lk *LogKeyboard) openDDWatchDocs(ctx context.Context, project *types.Project) {
	eg.Go(tracing.EventWrapFuncForErrGroup(ctx, "menu/gui/watch", tracing.SpanOptions{},
		func(ctx context.Context) error {
			link := fmt.Sprintf("docker-desktop://dashboard/docker-compose/%s/watch", project.Name)
			err := open.Run(link)
			if err != nil {
				err = fmt.Errorf("Could not open Docker Desktop Compose UI")
				lk.keyboardError("Watch Docs", err)
			}
			return err
		}),
	)
}

func (lk *LogKeyboard) keyboardError(prefix string, err error) {
	lk.kError.addError(prefix, err)

	lk.printNavigationMenu()
	timer1 := time.NewTimer((DISPLAY_ERROR_TIME + 1) * time.Second)
	go func() {
		<-timer1.C
		lk.printNavigationMenu()
	}()
}

func (lk *LogKeyboard) StartWatch(ctx context.Context, doneCh chan bool, project *types.Project, options api.UpOptions) {
	if !lk.IsWatchConfigured {
		if lk.IsDDComposeUIActive {
			// we try to open watch docs
			lk.openDDWatchDocs(ctx, project)
		}
		// either way we mark menu/watch as an error
		eg.Go(tracing.EventWrapFuncForErrGroup(ctx, "menu/watch", tracing.SpanOptions{},
			func(ctx context.Context) error {
				err := fmt.Errorf("Watch is not yet configured. Learn more: %s", ansiColor(CYAN, "https://docs.docker.com/compose/file-watch/"))
				lk.keyboardError("Watch", err)
				return err
			}))
		return

	}
	lk.Watch.switchWatching()
	if !lk.Watch.isWatching() {
		lk.Watch.Cancel()
	} else {
		eg.Go(tracing.EventWrapFuncForErrGroup(ctx, "menu/watch", tracing.SpanOptions{},
			func(ctx context.Context) error {
				if options.Create.Build == nil {
					err := fmt.Errorf("Cannot run watch mode with flag --no-build")
					lk.keyboardError("Watch", err)
					return err
				}

				lk.Watch.newContext(ctx)
				buildOpts := *options.Create.Build
				buildOpts.Quiet = true
				return lk.Watch.WatchFn(lk.Watch.Ctx, doneCh, project, options.Start.Services, api.WatchOptions{
					Build: &buildOpts,
					LogTo: options.Start.Attach,
				})
			}))
	}
}

func (lk *LogKeyboard) HandleKeyEvents(event keyboard.KeyEvent, ctx context.Context, doneCh chan bool, project *types.Project, options api.UpOptions) {
	switch kRune := event.Rune; kRune {
	case 'v':
		lk.openDockerDesktop(ctx, project)
	case 'w':
		lk.StartWatch(ctx, doneCh, project, options)
	case 'o':
		lk.openDDComposeUI(ctx, project)
	}
	switch key := event.Key; key {
	case keyboard.KeyCtrlC:
		_ = keyboard.Close()
		lk.clearNavigationMenu()
		ShowCursor()

		lk.logLevel = NONE
		if lk.Watch.Watching && lk.Watch.Cancel != nil {
			lk.Watch.Cancel()
			_ = eg.Wait().ErrorOrNil() // Need to print this ?
		}
		// will notify main thread to kill and will handle gracefully
		lk.signalChannel <- syscall.SIGINT
	case keyboard.KeyEnter:
		lk.printNavigationMenu()
	}
}

func allocateSpace(lines int) {
	for i := 0; i < lines; i++ {
		ClearLine()
		NewLine()
		MoveCursorX(0)
	}
}

func extraLines(s string) int {
	return int(math.Floor(float64(lenAnsi(s)) / float64(goterm.Width())))
}

func shortcutKeyColor(key string) string {
	foreground := "38;2"
	black := "0;0;0"
	background := "48;2"
	white := "255;255;255"
	return ansiColor(foreground+";"+black+";"+background+";"+white, key, BOLD)
}

func navColor(key string) string {
	return ansiColor(FAINT, key)
}
