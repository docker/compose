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
	"encoding/json"
	"testing"

	"github.com/docker/cli/cli-plugins/hooks"
	"gotest.tools/v3/assert"
)

func TestHandleHook_NoArgs(t *testing.T) {
	var buf bytes.Buffer
	err := handleHook(nil, &buf)
	assert.NilError(t, err)
	assert.Equal(t, buf.String(), "")
}

func TestHandleHook_InvalidJSON(t *testing.T) {
	var buf bytes.Buffer
	err := handleHook([]string{"not json"}, &buf)
	assert.NilError(t, err)
	assert.Equal(t, buf.String(), "")
}

func TestHandleHook_UnknownCommand(t *testing.T) {
	data := marshalHookData(t, hooks.Request{
		RootCmd: "compose push",
	})
	var buf bytes.Buffer
	err := handleHook([]string{data}, &buf)
	assert.NilError(t, err)
	assert.Equal(t, buf.String(), "")
}

func TestHandleHook_LogsCommand(t *testing.T) {
	tests := []struct {
		rootCmd  string
		wantHint string
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
			err := handleHook([]string{data}, &buf)
			assert.NilError(t, err)

			msg := unmarshalResponse(t, buf.Bytes())
			assert.Equal(t, msg.Type, hooks.NextSteps)
			assert.Equal(t, msg.Template, tt.wantHint)
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
			err := handleHook([]string{data}, &buf)
			assert.NilError(t, err)

			if tt.wantHint {
				msg := unmarshalResponse(t, buf.Bytes())
				assert.Equal(t, msg.Template, composeLogsHint)
			} else {
				assert.Equal(t, buf.String(), "")
			}
		})
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
