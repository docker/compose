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
	"fmt"
	"strings"
	"time"

	"github.com/buger/goterm"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
)

type Stopping struct {
	api.LogConsumer
	enabled   bool
	spinner   *progress.Spinner
	ticker    *time.Ticker
	startedAt time.Time
}

func NewStopping(l api.LogConsumer) *Stopping {
	s := &Stopping{}
	s.LogConsumer = logDecorator{
		decorated: l,
		Before:    s.clear,
		After:     s.print,
	}
	return s
}

func (s *Stopping) ApplicationTermination() {
	if progress.Mode != progress.ModeAuto {
		// User explicitly opted for output format
		return
	}
	if disableAnsi {
		return
	}
	s.enabled = true
	s.spinner = progress.NewSpinner()
	hideCursor()
	s.startedAt = time.Now()
	s.ticker = time.NewTicker(100 * time.Millisecond)
	go func() {
		for {
			<-s.ticker.C
			s.print()
		}
	}()
}

func (s *Stopping) Close() {
	showCursor()
	if s.ticker != nil {
		s.ticker.Stop()
	}
	s.clear()
}

func (s *Stopping) clear() {
	if !s.enabled {
		return
	}

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

const stoppingBanner = "Gracefully Stopping... (press Ctrl+C again to force)"

func (s *Stopping) print() {
	if !s.enabled {
		return
	}

	height := goterm.Height()
	width := goterm.Width()
	carriageReturn()
	saveCursor()

	moveCursor(height, 0)
	clearLine()
	elapsed := time.Since(s.startedAt).Seconds()
	timer := fmt.Sprintf("%.1fs ", elapsed)
	pad := width - len(timer) - len(stoppingBanner) - 5
	fmt.Printf("%s %s %s %s",
		progress.CountColor(s.spinner.String()),
		stoppingBanner,
		strings.Repeat(" ", pad),
		progress.TimerColor(timer),
	)

	carriageReturn()
	restoreCursor()
}
