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
	"strings"
	"syscall"
	"time"

	"github.com/buger/goterm"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/internal/tracing"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/eiannone/keyboard"
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

		moveCursor(height-1-extraLines(info)-extraLines(errMessage), 0)
		clearLine()

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
	Watching bool
	Watcher  Feature
}

// Feature is an compose feature that can be started/stopped by a menu command
type Feature interface {
	Start(context.Context) error
	Stop() error
}

type KEYBOARD_LOG_LEVEL int

const (
	NONE  KEYBOARD_LOG_LEVEL = 0
	INFO  KEYBOARD_LOG_LEVEL = 1
	DEBUG KEYBOARD_LOG_LEVEL = 2
)

type LogKeyboard struct {
	kError                KeyboardError
	Watch                 *KeyboardWatch
	Detach                func()
	IsDockerDesktopActive bool
	logLevel              KEYBOARD_LOG_LEVEL
	signalChannel         chan<- os.Signal
}

func NewKeyboardManager(isDockerDesktopActive bool, sc chan<- os.Signal) *LogKeyboard {
	return &LogKeyboard{
		IsDockerDesktopActive: isDockerDesktopActive,
		logLevel:              INFO,
		signalChannel:         sc,
	}
}

func (lk *LogKeyboard) Decorate(l api.LogConsumer) api.LogConsumer {
	return logDecorator{
		decorated: l,
		Before:    lk.clearNavigationMenu,
		After:     lk.PrintKeyboardInfo,
	}
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
		moveCursorUp(lines)
	}
}

func (lk *LogKeyboard) printNavigationMenu() {
	offset := 1
	lk.clearNavigationMenu()
	lk.createBuffer(offset)

	if lk.logLevel == INFO {
		height := goterm.Height()
		menu := lk.navigationMenu()

		carriageReturn()
		saveCursor()

		lk.kError.printError(height, menu)

		moveCursor(height-extraLines(menu), 0)
		clearLine()
		fmt.Print(menu)

		carriageReturn()
		restoreCursor()
	}
}

func (lk *LogKeyboard) navigationMenu() string {
	var items []string
	if lk.IsDockerDesktopActive {
		items = append(items, shortcutKeyColor("v")+navColor(" View in Docker Desktop"))
	}

	if lk.IsDockerDesktopActive {
		items = append(items, shortcutKeyColor("o")+navColor(" View Config"))
	}

	isEnabled := " Enable"
	if lk.Watch != nil && lk.Watch.Watching {
		isEnabled = " Disable"
	}
	items = append(items, shortcutKeyColor("w")+navColor(isEnabled+" Watch"))
	items = append(items, shortcutKeyColor("d")+navColor(" Detach"))

	return strings.Join(items, "   ")
}

func (lk *LogKeyboard) clearNavigationMenu() {
	height := goterm.Height()
	carriageReturn()
	saveCursor()

	// clearLine()
	for i := 0; i < height; i++ {
		moveCursorDown(1)
		clearLine()
	}
	restoreCursor()
}

func (lk *LogKeyboard) openDockerDesktop(ctx context.Context, project *types.Project) {
	if !lk.IsDockerDesktopActive {
		return
	}
	go func() {
		_ = tracing.EventWrapFuncForErrGroup(ctx, "menu/gui", tracing.SpanOptions{},
			func(ctx context.Context) error {
				link := fmt.Sprintf("docker-desktop://dashboard/apps/%s", project.Name)
				err := open.Run(link)
				if err != nil {
					err = fmt.Errorf("could not open Docker Desktop")
					lk.keyboardError("View", err)
				}
				return err
			})()
	}()
}

func (lk *LogKeyboard) openDDComposeUI(ctx context.Context, project *types.Project) {
	if !lk.IsDockerDesktopActive {
		return
	}
	go func() {
		_ = tracing.EventWrapFuncForErrGroup(ctx, "menu/gui/composeview", tracing.SpanOptions{},
			func(ctx context.Context) error {
				link := fmt.Sprintf("docker-desktop://dashboard/docker-compose/%s", project.Name)
				err := open.Run(link)
				if err != nil {
					err = fmt.Errorf("could not open Docker Desktop Compose UI")
					lk.keyboardError("View Config", err)
				}
				return err
			})()
	}()
}

func (lk *LogKeyboard) openDDWatchDocs(ctx context.Context, project *types.Project) {
	go func() {
		_ = tracing.EventWrapFuncForErrGroup(ctx, "menu/gui/watch", tracing.SpanOptions{},
			func(ctx context.Context) error {
				link := fmt.Sprintf("docker-desktop://dashboard/docker-compose/%s/watch", project.Name)
				err := open.Run(link)
				if err != nil {
					err = fmt.Errorf("could not open Docker Desktop Compose UI")
					lk.keyboardError("Watch Docs", err)
				}
				return err
			})()
	}()
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

func (lk *LogKeyboard) ToggleWatch(ctx context.Context, options api.UpOptions) {
	if lk.Watch == nil {
		return
	}
	if lk.Watch.Watching {
		err := lk.Watch.Watcher.Stop()
		if err != nil {
			options.Start.Attach.Err(api.WatchLogger, err.Error())
		} else {
			lk.Watch.Watching = false
		}
	} else {
		go func() {
			_ = tracing.EventWrapFuncForErrGroup(ctx, "menu/watch", tracing.SpanOptions{},
				func(ctx context.Context) error {
					err := lk.Watch.Watcher.Start(ctx)
					if err != nil {
						options.Start.Attach.Err(api.WatchLogger, err.Error())
					} else {
						lk.Watch.Watching = true
					}
					return err
				})()
		}()
	}
}

func (lk *LogKeyboard) HandleKeyEvents(ctx context.Context, event keyboard.KeyEvent, project *types.Project, options api.UpOptions) {
	switch kRune := event.Rune; kRune {
	case 'd':
		lk.clearNavigationMenu()
		lk.Detach()
	case 'v':
		lk.openDockerDesktop(ctx, project)
	case 'w':
		if lk.Watch == nil {
			// we try to open watch docs if DD is installed
			if lk.IsDockerDesktopActive {
				lk.openDDWatchDocs(ctx, project)
			}
			// either way we mark menu/watch as an error
			go func() {
				_ = tracing.EventWrapFuncForErrGroup(ctx, "menu/watch", tracing.SpanOptions{},
					func(ctx context.Context) error {
						err := fmt.Errorf("watch is not yet configured. Learn more: %s", ansiColor(CYAN, "https://docs.docker.com/compose/file-watch/"))
						lk.keyboardError("Watch", err)
						return err
					})()
			}()
		}
		lk.ToggleWatch(ctx, options)
	case 'o':
		lk.openDDComposeUI(ctx, project)
	}
	switch key := event.Key; key {
	case keyboard.KeyCtrlC:
		_ = keyboard.Close()
		lk.clearNavigationMenu()
		showCursor()

		lk.logLevel = NONE
		// will notify main thread to kill and will handle gracefully
		lk.signalChannel <- syscall.SIGINT
	case keyboard.KeyCtrlZ:
		handleCtrlZ()
	case keyboard.KeyEnter:
		newLine()
		lk.printNavigationMenu()
	}
}

func (lk *LogKeyboard) EnableWatch(enabled bool, watcher Feature) {
	lk.Watch = &KeyboardWatch{
		Watching: enabled,
		Watcher:  watcher,
	}
}

func (lk *LogKeyboard) EnableDetach(detach func()) {
	lk.Detach = detach
}

func allocateSpace(lines int) {
	for i := 0; i < lines; i++ {
		clearLine()
		newLine()
		carriageReturn()
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
