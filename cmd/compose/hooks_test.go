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
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/cli/cli-plugins/hooks"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/cmd/formatter"
	"github.com/docker/compose/v5/internal/desktop"
)

const testDeepLink = "docker-desktop://dashboard/logs"

// TestMain stubs the Docker Desktop feature-flag check and the project
// loader so handleHook tests don't make a live engine call or read a
// compose file from the test runner's working directory. Individual tests
// override either stub with t.Cleanup to restore.
func TestMain(m *testing.M) {
	logsTabEnabled = func(context.Context) bool { return true }
	resolveAppID = func(context.Context, map[string]string) string { return "" }
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
		wantHint func(appID string) string
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
			assert.Equal(t, msg.Template, tt.wantHint(""))
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
				assert.Equal(t, msg.Template, composeLogsHint(""))
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
	wantLink := formatter.OSC8Link(testDeepLink, testDeepLink)
	assert.Assert(t, len(wantLink) > len(testDeepLink), "OSC8Link should wrap the URL with escape sequences")
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
	assert.Assert(t, strings.Contains(msg.Template, testDeepLink), "hint should contain the deep link URL")
	assert.Assert(t, !strings.Contains(msg.Template, "\033"), "hint should not contain ANSI escape sequences")
}

func TestHandleHook_AppIDEncodedInURL(t *testing.T) {
	prev := resolveAppID
	t.Cleanup(func() { resolveAppID = prev })
	resolveAppID = func(context.Context, map[string]string) string { return "myapp" }

	t.Setenv("NO_COLOR", "1") // emit a plain URL we can substring-match
	for _, rootCmd := range []string{"compose logs", "compose up"} {
		t.Run(rootCmd, func(t *testing.T) {
			data := marshalHookData(t, hooks.Request{
				RootCmd: rootCmd,
				Flags:   map[string]string{"d": "true"},
			})
			var buf bytes.Buffer
			err := handleHook(t.Context(), []string{data}, &buf)
			assert.NilError(t, err)

			msg := unmarshalResponse(t, buf.Bytes())
			assert.Assert(t, strings.Contains(msg.Template, desktop.BuildLogsURL("myapp")),
				"hint should include the project-scoped URL, got %q", msg.Template)
		})
	}
}

func TestHandleHook_DockerLogsIgnoresAppID(t *testing.T) {
	// resolveAppID is not consulted for "logs" because that hint isn't
	// resolveProject; assert the URL stays paramless even if a stub
	// would otherwise return a value.
	prev := resolveAppID
	t.Cleanup(func() { resolveAppID = prev })
	resolveAppID = func(context.Context, map[string]string) string {
		t.Fatalf("resolveAppID should not be called for docker logs")
		return ""
	}

	t.Setenv("NO_COLOR", "1")
	data := marshalHookData(t, hooks.Request{RootCmd: "logs"})
	var buf bytes.Buffer
	err := handleHook(t.Context(), []string{data}, &buf)
	assert.NilError(t, err)

	msg := unmarshalResponse(t, buf.Bytes())
	assert.Assert(t, strings.Contains(msg.Template, testDeepLink),
		"docker logs hint should contain the bare deep link")
	assert.Assert(t, !strings.Contains(msg.Template, "?appId="),
		"docker logs hint must not encode an appId")
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
	assert.Assert(t, strings.Contains(msg.Template, testDeepLink), "hint should contain the deep link URL")
	assert.Assert(t, !strings.Contains(msg.Template, "\033"), "hint should not contain ANSI escape sequences")
}

func TestResolveAppID_ShortCircuitsOnFlag(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
	}{
		{name: "long --project-name", flags: map[string]string{"project-name": ""}},
		{name: "short -p", flags: map[string]string{"p": ""}},
		{name: "long --file", flags: map[string]string{"file": ""}},
		{name: "short -f", flags: map[string]string{"f": ""}},
		{name: "long --project-directory", flags: map[string]string{"project-directory": ""}},
		{name: "deprecated --workdir alias", flags: map[string]string{"workdir": ""}},
		{name: "long --env-file", flags: map[string]string{"env-file": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a real tmpdir as workDir so the short-circuit path is
			// exercised independently of the loader's file discovery.
			got := resolveAppIDIn(t.Context(), tt.flags, t.TempDir())
			assert.Equal(t, got, "")
		})
	}
}

func TestResolveAppID_NameFromComposeFile(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "compose.yaml", "name: from-yaml\nservices:\n  svc:\n    image: nginx\n")
	unsetEnv(t, "COMPOSE_PROJECT_NAME")
	unsetEnv(t, "COMPOSE_FILE")

	got := resolveAppIDIn(t.Context(), nil, dir)
	assert.Equal(t, got, "from-yaml")
}

func TestResolveAppID_EnvVarOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "compose.yaml", "name: from-yaml\nservices:\n  svc:\n    image: nginx\n")
	t.Setenv("COMPOSE_PROJECT_NAME", "from-env")
	unsetEnv(t, "COMPOSE_FILE")

	got := resolveAppIDIn(t.Context(), nil, dir)
	assert.Equal(t, got, "from-env")
}

func TestResolveAppID_NoComposeFileReturnsEmpty(t *testing.T) {
	unsetEnv(t, "COMPOSE_PROJECT_NAME")
	unsetEnv(t, "COMPOSE_FILE")

	got := resolveAppIDIn(t.Context(), nil, t.TempDir())
	assert.Equal(t, got, "")
}

// unsetEnv removes an env var for the lifetime of the test, restoring its
// prior state on cleanup. t.Setenv("", "") is not equivalent to unset:
// compose-go's WithConfigFileEnv treats empty as a meaningful override.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if !had {
			return
		}
		if err := os.Setenv(key, prev); err != nil {
			t.Errorf("restore env %s: %v", key, err)
		}
	})
}

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
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
