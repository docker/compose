//go:build !windows

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
	"fmt"
	"os"
	"testing"

	"github.com/creack/pty"
	"github.com/docker/cli/cli/streams"
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/cmd/display"
	"github.com/docker/compose/v5/pkg/mocks"
)

// saveGlobalState snapshots package-level state that selectEventProcessor
// mutates (display.Mode and, in JSON mode, the logrus standard formatter)
// and restores it on test cleanup.
func saveGlobalState(t *testing.T) {
	t.Helper()
	originalMode := display.Mode
	originalFormatter := logrus.StandardLogger().Formatter
	t.Cleanup(func() {
		display.Mode = originalMode
		logrus.SetFormatter(originalFormatter)
	})
}

// newStream returns a *streams.Out whose IsTerminal() matches tty. When tty is
// true it is backed by a pseudo-terminal slave; otherwise by an os.Pipe writer.
func newStream(t *testing.T, tty bool) *streams.Out {
	t.Helper()
	if tty {
		ptmx, slave, err := pty.Open()
		assert.NilError(t, err)
		t.Cleanup(func() {
			_ = ptmx.Close()
			_ = slave.Close()
		})
		return streams.NewOut(slave)
	}
	r, w, err := os.Pipe()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	return streams.NewOut(w)
}

func newMockCli(t *testing.T, out, errStream *streams.Out) *mocks.MockCli {
	t.Helper()
	cli := mocks.NewMockCli(gomock.NewController(t))
	cli.EXPECT().Out().Return(out).AnyTimes()
	cli.EXPECT().Err().Return(errStream).AnyTimes()
	return cli
}

// TestSelectEventProcessor_AutoMode covers the regression from docker/compose#13570:
// auto mode must probe Err() (not Out()) so `docker compose up | tee log` still
// renders the colorized UI on stderr.
func TestSelectEventProcessor_AutoMode(t *testing.T) {
	tests := []struct {
		name     string
		outIsTTY bool
		errIsTTY bool
		ansi     string
		wantType string
	}{
		{
			name:     "stderr TTY, stdout piped -> Full",
			errIsTTY: true,
			ansi:     "auto",
			wantType: "*display.ttyWriter",
		},
		{
			name:     "stderr piped, stdout TTY -> Plain (do not fall back to stdout)",
			outIsTTY: true,
			ansi:     "auto",
			wantType: "*display.plainWriter",
		},
		{
			name:     "both TTY -> Full",
			outIsTTY: true,
			errIsTTY: true,
			ansi:     "auto",
			wantType: "*display.ttyWriter",
		},
		{
			name:     "both piped -> Plain",
			ansi:     "auto",
			wantType: "*display.plainWriter",
		},
		{
			name:     "ansi never forces Plain even when stderr is TTY",
			outIsTTY: true,
			errIsTTY: true,
			ansi:     "never",
			wantType: "*display.plainWriter",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saveGlobalState(t)
			cli := newMockCli(t, newStream(t, tc.outIsTTY), newStream(t, tc.errIsTTY))

			ep, err := selectEventProcessor(cli, "", tc.ansi, false)
			assert.NilError(t, err)
			assert.Equal(t, fmt.Sprintf("%T", ep), tc.wantType)
		})
	}
}

func TestSelectEventProcessor_ExplicitMode(t *testing.T) {
	tests := []struct {
		name        string
		progress    string
		ansi        string
		wantType    string
		wantErrText string
	}{
		{
			name:     "progress=tty forces Full regardless of streams",
			progress: display.ModeTTY,
			ansi:     "auto",
			wantType: "*display.ttyWriter",
		},
		{
			name:        "progress=tty with ansi=never is rejected",
			progress:    display.ModeTTY,
			ansi:        "never",
			wantErrText: "can't use --progress tty while ANSI support is disabled",
		},
		{
			name:     "progress=plain forces Plain",
			progress: display.ModePlain,
			ansi:     "auto",
			wantType: "*display.plainWriter",
		},
		{
			name:        "progress=plain with ansi=always is rejected",
			progress:    display.ModePlain,
			ansi:        "always",
			wantErrText: "can't use --progress plain while ANSI support is forced",
		},
		{
			name:     "progress=quiet returns Quiet",
			progress: display.ModeQuiet,
			ansi:     "auto",
			wantType: "*display.quiet",
		},
		{
			name:     `progress="none" aliases to Quiet`,
			progress: "none",
			ansi:     "auto",
			wantType: "*display.quiet",
		},
		{
			name:     "progress=json returns JSON",
			progress: display.ModeJSON,
			ansi:     "auto",
			wantType: "*display.jsonWriter",
		},
		{
			name:        "unknown progress value is rejected",
			progress:    "bogus",
			ansi:        "auto",
			wantErrText: `unsupported --progress value "bogus"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saveGlobalState(t)
			// Explicit modes don't probe IsTerminal; pipes are fine for both.
			cli := newMockCli(t, newStream(t, false), newStream(t, false))

			ep, err := selectEventProcessor(cli, tc.progress, tc.ansi, false)
			if tc.wantErrText != "" {
				assert.ErrorContains(t, err, tc.wantErrText)
				assert.Assert(t, ep == nil)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, fmt.Sprintf("%T", ep), tc.wantType)
		})
	}
}
