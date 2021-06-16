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
	"os"
	gosignal "os/signal"
	"runtime"
	"time"

	"github.com/buger/goterm"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/signal"
)

func (s *composeService) monitorTTySize(ctx context.Context, container string, resize func(context.Context, string, moby.ResizeOptions) error) {
	err := resize(ctx, container, moby.ResizeOptions{ // nolint:errcheck
		Height: uint(goterm.Height()),
		Width:  uint(goterm.Width()),
	})
	if err != nil {
		return
	}

	sigchan := make(chan os.Signal, 1)
	gosignal.Notify(sigchan, signal.SIGWINCH)

	if runtime.GOOS == "windows" {
		// Windows has no SIGWINCH support, so we have to poll tty size ¯\_(ツ)_/¯
		go func() {
			prevH := goterm.Height()
			prevW := goterm.Width()
			for {
				time.Sleep(time.Millisecond * 250)
				h := goterm.Height()
				w := goterm.Width()
				if prevW != w || prevH != h {
					sigchan <- signal.SIGWINCH
				}
				prevH = h
				prevW = w
			}
		}()
	}

	go func() {
		for {
			select {
			case <-sigchan:
				resize(ctx, container, moby.ResizeOptions{ // nolint:errcheck
					Height: uint(goterm.Height()),
					Width:  uint(goterm.Width()),
				})
			case <-ctx.Done():
				return
			}
		}
	}()
}
