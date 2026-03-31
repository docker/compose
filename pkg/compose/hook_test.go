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
	"net"
	"os"
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
