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
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/creack/pty"
	"github.com/docker/cli/cli/streams"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// TestRunHook_ConsoleSize verifies that ConsoleSize is only passed to ExecAttach
// when the service has TTY enabled. When TTY is disabled, passing a non-zero
// ConsoleSize causes the Docker daemon to return "console size is only supported
// when TTY is enabled" (regression introduced in v5.1.0).
func TestRunHook_ConsoleSize(t *testing.T) {
	tests := []struct {
		name            string
		tty             bool
		expectedConsole client.ConsoleSize
	}{
		{
			name:            "no tty - ConsoleSize must be zero",
			tty:             false,
			expectedConsole: client.ConsoleSize{},
		},
		{
			name:            "with tty - ConsoleSize should reflect terminal dimensions",
			tty:             true,
			expectedConsole: client.ConsoleSize{Width: 80, Height: 24},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockAPI := mocks.NewMockAPIClient(mockCtrl)
			mockCli := mocks.NewMockCli(mockCtrl)
			mockCli.EXPECT().Client().Return(mockAPI).AnyTimes()
			mockCli.EXPECT().Err().Return(streams.NewOut(os.Stderr)).AnyTimes()

			// Create a PTY so GetTtySize() returns real non-zero dimensions,
			// simulating an interactive terminal session.
			ptmx, tty, err := pty.Open()
			assert.NilError(t, err)
			t.Cleanup(func() {
				_ = ptmx.Close()
				_ = tty.Close()
			})
			assert.NilError(t, pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80}))
			mockCli.EXPECT().Out().Return(streams.NewOut(tty)).AnyTimes()

			service := types.ServiceConfig{
				Name: "test",
				Tty:  tc.tty,
			}
			hook := types.ServiceHook{Command: []string{"echo", "hello"}}
			ctr := container.Summary{ID: "container123"}

			mockAPI.EXPECT().
				ExecCreate(gomock.Any(), "container123", gomock.Any()).
				Return(client.ExecCreateResult{ID: "exec123"}, nil)

			// Return a pipe that immediately closes so the reader gets EOF.
			serverConn, clientConn := net.Pipe()
			serverConn.Close() //nolint:errcheck
			mockAPI.EXPECT().
				ExecAttach(gomock.Any(), "exec123", client.ExecAttachOptions{
					TTY:         tc.tty,
					ConsoleSize: tc.expectedConsole,
				}).
				Return(client.ExecAttachResult{
					HijackedResponse: client.NewHijackedResponse(clientConn, ""),
				}, nil)

			mockAPI.EXPECT().
				ExecInspect(gomock.Any(), "exec123", gomock.Any()).
				Return(client.ExecInspectResult{ExitCode: 0}, nil)

			s, err := NewComposeService(mockCli)
			assert.NilError(t, err)

			noopListener := func(api.ContainerEvent) {}
			err = s.(*composeService).runHook(t.Context(), ctr, service, hook, noopListener)
			assert.NilError(t, err)
		})
	}
}

// writeStdcopyFrame writes payload as one multiplexed frame, the format
// stdcopy.StdCopy reads when the service has no TTY.
func writeStdcopyFrame(w io.Writer, stream byte, payload string) error {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))

	if _, err := w.Write(header); err != nil {
		return err
	}

	_, err := io.WriteString(w, payload)

	return err
}

// TestRunHook_FailureIncludesOutput verifies that the error of a failing hook
// carries the tail of what the hook printed. The exit code alone does not say
// why it failed, and for a detached hook the output used to be discarded by the
// daemon, leaving no way to find out at all.
func TestRunHook_FailureIncludesOutput(t *testing.T) {
	tests := []struct {
		name     string
		listener api.ContainerEventListener
	}{
		{name: "with listener", listener: func(api.ContainerEvent) {}},
		{name: "without listener", listener: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockAPI := mocks.NewMockAPIClient(mockCtrl)
			mockCli := mocks.NewMockCli(mockCtrl)
			mockCli.EXPECT().Client().Return(mockAPI).AnyTimes()
			mockCli.EXPECT().Err().Return(streams.NewOut(os.Stderr)).AnyTimes()
			mockCli.EXPECT().Out().Return(streams.NewOut(os.Stdout)).AnyTimes()

			service := types.ServiceConfig{Name: "db"}
			hook := types.ServiceHook{Command: []string{"/migrate.sh"}}
			ctr := container.Summary{ID: "container123"}

			// Both streams must be attached, otherwise the daemon drops the output.
			mockAPI.EXPECT().
				ExecCreate(gomock.Any(), "container123", gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, options client.ExecCreateOptions) (client.ExecCreateResult, error) {
					assert.Check(t, options.AttachStdout, "stdout must be attached")
					assert.Check(t, options.AttachStderr, "stderr must be attached")

					return client.ExecCreateResult{ID: "exec123"}, nil
				})

			serverConn, clientConn := net.Pipe()

			go func() {
				assert.NilError(t, writeStdcopyFrame(serverConn, 1, "migrating\n"))
				assert.NilError(t, writeStdcopyFrame(serverConn, 2, "SQLSTATE[42S02]: table not found\n"))
				serverConn.Close() //nolint:errcheck
			}()

			mockAPI.EXPECT().
				ExecAttach(gomock.Any(), "exec123", gomock.Any()).
				Return(client.ExecAttachResult{
					HijackedResponse: client.NewHijackedResponse(clientConn, ""),
				}, nil)

			mockAPI.EXPECT().
				ExecInspect(gomock.Any(), "exec123", gomock.Any()).
				Return(client.ExecInspectResult{ExitCode: 1}, nil)

			s, err := NewComposeService(mockCli)
			assert.NilError(t, err)

			err = s.(*composeService).runHook(t.Context(), ctr, service, hook, tc.listener)
			assert.ErrorContains(t, err, "db hook exited with status 1")
			assert.ErrorContains(t, err, "SQLSTATE[42S02]: table not found")
		})
	}
}

// TestRunHook_SuccessKeepsOutputOut verifies a successful hook stays silent: its
// output belongs to the listener, not to an error nobody returns.
func TestRunHook_SuccessKeepsOutputOut(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mocks.NewMockAPIClient(mockCtrl)
	mockCli := mocks.NewMockCli(mockCtrl)
	mockCli.EXPECT().Client().Return(mockAPI).AnyTimes()
	mockCli.EXPECT().Err().Return(streams.NewOut(os.Stderr)).AnyTimes()
	mockCli.EXPECT().Out().Return(streams.NewOut(os.Stdout)).AnyTimes()

	var lines []string

	listener := func(event api.ContainerEvent) {
		lines = append(lines, event.Line)
	}

	serverConn, clientConn := net.Pipe()

	go func() {
		assert.NilError(t, writeStdcopyFrame(serverConn, 1, "all good\n"))
		serverConn.Close() //nolint:errcheck
	}()

	mockAPI.EXPECT().
		ExecCreate(gomock.Any(), "container123", gomock.Any()).
		Return(client.ExecCreateResult{ID: "exec123"}, nil)
	mockAPI.EXPECT().
		ExecAttach(gomock.Any(), "exec123", gomock.Any()).
		Return(client.ExecAttachResult{
			HijackedResponse: client.NewHijackedResponse(clientConn, ""),
		}, nil)
	mockAPI.EXPECT().
		ExecInspect(gomock.Any(), "exec123", gomock.Any()).
		Return(client.ExecInspectResult{ExitCode: 0}, nil)

	s, err := NewComposeService(mockCli)
	assert.NilError(t, err)

	err = s.(*composeService).runHook(t.Context(),
		container.Summary{ID: "container123"},
		types.ServiceConfig{Name: "db"},
		types.ServiceHook{Command: []string{"/migrate.sh"}},
		listener)
	assert.NilError(t, err)
	assert.DeepEqual(t, lines, []string{"all good"})
}

func TestOutputTail(t *testing.T) {
	t.Run("keeps only the last lines", func(t *testing.T) {
		tail := newOutputTail(2, 1024)
		_, err := tail.Write([]byte("one\ntwo\nthree\n"))
		assert.NilError(t, err)
		assert.Equal(t, tail.String(), "two\nthree")
	})

	t.Run("keeps a line without trailing newline", func(t *testing.T) {
		tail := newOutputTail(5, 1024)
		_, err := tail.Write([]byte("no newline here"))
		assert.NilError(t, err)
		assert.Equal(t, tail.String(), "no newline here")
	})

	t.Run("drops blank lines", func(t *testing.T) {
		tail := newOutputTail(3, 1024)
		_, err := tail.Write([]byte("\n\nreal\n\n"))
		assert.NilError(t, err)
		assert.Equal(t, tail.String(), "real")
	})

	t.Run("bounds bytes and keeps accepting writes", func(t *testing.T) {
		tail := newOutputTail(10, 16)

		n, err := tail.Write([]byte(strings.Repeat("x", 4096)))
		assert.NilError(t, err)
		assert.Equal(t, n, 4096, "a full buffer must not short-write, it would block the hook")

		_, err = tail.Write([]byte("\nlast line\n"))
		assert.NilError(t, err)
		assert.Check(t, len(tail.String()) <= 16, "output must stay within the byte limit")
		assert.Check(t, strings.HasSuffix(tail.String(), "last line"), "the newest output must survive")
	})
}
