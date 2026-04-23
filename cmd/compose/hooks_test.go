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
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/docker/cli/cli-plugins/hooks"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/cmd/formatter"
)

// TestMain stubs the Docker Desktop feature-flag check so handleHook tests
// don't attempt a live engine call. Individual tests can still override
// isFeatureEnabled with their own stub + t.Cleanup to restore.
func TestMain(m *testing.M) {
	logsTabEnabled = func(context.Context) bool { return true }
	os.Exit(m.Run())
}

func TestHandleHook_NoArgs(t *testing.T) {
	var buf bytes.Buffer
	err := handleHook(t.Context(), nil, &buf)
	assert.NilError(t, err)
	assert.Equal(t, buf.String(), "")
}

func TestHandleHook_InvalidJSON(t *testing.T) {
	var buf bytes.Buffer
	err := handleHook(t.Context(), []string{"not json"}, &buf)
	assert.NilError(t, err)
	assert.Equal(t, buf.String(), "")
}

func TestHandleHook_UnknownCommand(t *testing.T) {
	data := marshalHookData(t, hooks.Request{
		RootCmd: "compose push",
	})
	var buf bytes.Buffer
	err := handleHook(t.Context(), []string{data}, &buf)
	assert.NilError(t, err)
	assert.Equal(t, buf.String(), "")
}

func TestHandleHook_LogsCommand(t *testing.T) {
	tests := []struct {
		rootCmd  string
		wantHint func() string
	}{
		{rootCmd: "compose logs", wantHint: composeLogsHint},
		{rootCmd: "logs", wantHint: dockerLogsHint},
	}
	for _, tt := range tests {
		t.Run(tt.rootCmd, func(t *testing.T) {
			data := marshalHookData(t, hooks.Request{
				RootCmd: tt.rootCmd,
			})
			var buf bytes.Buffer
			err := handleHook(t.Context(), []string{data}, &buf)
			assert.NilError(t, err)

			msg := unmarshalResponse(t, buf.Bytes())
			assert.Equal(t, msg.Type, hooks.NextSteps)
			assert.Equal(t, msg.Template, tt.wantHint())
		})
	}
}

func TestHandleHook_ComposeUpDetached(t *testing.T) {
	tests := []struct {
		name     string
		flags    map[string]string
		wantHint bool
	}{
		{
			name:     "with --detach flag",
			flags:    map[string]string{"detach": ""},
			wantHint: true,
		},
		{
			name:     "with -d flag",
			flags:    map[string]string{"d": ""},
			wantHint: true,
		},
		{
			name:     "without detach flag",
			flags:    map[string]string{"build": ""},
			wantHint: false,
		},
		{
			name:     "no flags",
			flags:    map[string]string{},
			wantHint: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := marshalHookData(t, hooks.Request{
				RootCmd: "compose up",
				Flags:   tt.flags,
			})
			var buf bytes.Buffer
			err := handleHook(t.Context(), []string{data}, &buf)
			assert.NilError(t, err)

			if tt.wantHint {
				msg := unmarshalResponse(t, buf.Bytes())
				assert.Equal(t, msg.Template, composeLogsHint())
			} else {
				assert.Equal(t, buf.String(), "")
			}
		})
	}
}

func TestHandleHook_HintContainsOSC8Link(t *testing.T) {
	// Ensure ANSI is not suppressed by the test runner environment
	t.Setenv("NO_COLOR", "")
	t.Setenv("COMPOSE_ANSI", "")
	data := marshalHookData(t, hooks.Request{
		RootCmd: "compose logs",
	})
	var buf bytes.Buffer
	err := handleHook(t.Context(), []string{data}, &buf)
	assert.NilError(t, err)

	msg := unmarshalResponse(t, buf.Bytes())
	// Verify the template contains the OSC 8 hyperlink sequence
	wantLink := formatter.OSC8Link(deepLink, deepLink)
	assert.Assert(t, len(wantLink) > len(deepLink), "OSC8Link should wrap the URL with escape sequences")
	assert.Assert(t, strings.Contains(msg.Template, wantLink), "hint should contain OSC 8 hyperlink")
}

func TestHandleHook_NoColorDisablesOsc8(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	data := marshalHookData(t, hooks.Request{
		RootCmd: "compose logs",
	})
	var buf bytes.Buffer
	err := handleHook(t.Context(), []string{data}, &buf)
	assert.NilError(t, err)

	msg := unmarshalResponse(t, buf.Bytes())
	// With NO_COLOR set, the hint should contain the plain URL without escape sequences
	assert.Assert(t, strings.Contains(msg.Template, deepLink), "hint should contain the deep link URL")
	assert.Assert(t, !strings.Contains(msg.Template, "\033"), "hint should not contain ANSI escape sequences")
}

func TestHandleHook_FeatureFlagDisabledSuppressesHint(t *testing.T) {
	prev := logsTabEnabled
	t.Cleanup(func() { logsTabEnabled = prev })
	logsTabEnabled = func(context.Context) bool { return false }

	for _, rootCmd := range []string{"compose logs", "logs"} {
		t.Run(rootCmd, func(t *testing.T) {
			data := marshalHookData(t, hooks.Request{RootCmd: rootCmd})
			var buf bytes.Buffer
			err := handleHook(t.Context(), []string{data}, &buf)
			assert.NilError(t, err)
			assert.Equal(t, buf.String(), "")
		})
	}
}

func TestHandleHook_ComposeAnsiNeverDisablesOsc8(t *testing.T) {
	t.Setenv("COMPOSE_ANSI", "never")
	data := marshalHookData(t, hooks.Request{
		RootCmd: "compose logs",
	})
	var buf bytes.Buffer
	err := handleHook(t.Context(), []string{data}, &buf)
	assert.NilError(t, err)

	msg := unmarshalResponse(t, buf.Bytes())
	assert.Assert(t, strings.Contains(msg.Template, deepLink), "hint should contain the deep link URL")
	assert.Assert(t, !strings.Contains(msg.Template, "\033"), "hint should not contain ANSI escape sequences")
}

func marshalHookData(t *testing.T, data hooks.Request) string {
	t.Helper()
	b, err := json.Marshal(data)
	assert.NilError(t, err)
	return string(b)
}

func unmarshalResponse(t *testing.T, data []byte) hooks.Response {
	t.Helper()
	var msg hooks.Response
	err := json.Unmarshal(data, &msg)
	assert.NilError(t, err)
	return msg
}
