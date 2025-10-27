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
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"
)

func TestJsonWriter_Event(t *testing.T) {
	var out bytes.Buffer
	w := &jsonWriter{
		out:    &out,
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

	var actual jsonMessage
	err := json.Unmarshal(out.Bytes(), &actual)
	assert.NilError(t, err)

	expected := jsonMessage{
		DryRun:   true,
		ID:       event.ID,
		ParentID: event.ParentID,
		Text:     event.Text,
		Status:   event.StatusText,
		Current:  event.Current,
		Total:    event.Total,
		Percent:  event.Percent,
	}
	assert.DeepEqual(t, expected, actual)
}
