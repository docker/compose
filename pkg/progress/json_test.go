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

package progress

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"
)

func TestJsonWriter_Event(t *testing.T) {
	var out bytes.Buffer
	w := &jsonWriter{
		out:    &out,
		done:   make(chan bool),
		dryRun: true,
	}

	event := Event{
		ID:         "service1",
		ParentID:   "project",
		Text:       "Creating",
		StatusText: "Working",
		Current:    50,
		Total:      100,
		Percent:    50,
	}
	w.Event(event)

	var msg jsonMessage
	err := json.Unmarshal(out.Bytes(), &msg)
	assert.NilError(t, err)

	assert.Equal(t, true, msg.DryRun)
	assert.Equal(t, false, msg.Tail)
	assert.Equal(t, "service1", msg.ID)
	assert.Equal(t, "project", msg.ParentID)
	assert.Equal(t, "Creating", msg.Text)
	assert.Equal(t, "Working", msg.Status)
	assert.Equal(t, int64(50), msg.Current)
	assert.Equal(t, int64(100), msg.Total)
	assert.Equal(t, 50, msg.Percent)
}

func TestJsonWriter_TailMsgf(t *testing.T) {
	var out bytes.Buffer
	w := &jsonWriter{
		out:    &out,
		done:   make(chan bool),
		dryRun: false,
	}

	go func() {
		_ = w.Start(context.Background())
	}()

	w.TailMsgf("hello %s", "world")

	w.Stop()

	var msg jsonMessage
	err := json.Unmarshal(out.Bytes(), &msg)
	assert.NilError(t, err)

	assert.Equal(t, false, msg.DryRun)
	assert.Equal(t, true, msg.Tail)
	assert.Equal(t, "hello world", msg.Text)
	assert.Equal(t, "", msg.ID)
	assert.Equal(t, "", msg.Status)
}