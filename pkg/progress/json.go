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
	"context"
	"encoding/json"
	"fmt"
	"io"
)

type jsonWriter struct {
	out    io.Writer
	done   chan bool
	dryRun bool
}

type jsonMessage struct {
	DryRun bool   `json:"dry-run,omitempty"`
	Tail   bool   `json:"tail,omitempty"`
	ID     string `json:"id,omitempty"`
	Text   string `json:"text,omitempty"`
	Status string `json:"status,omitempty"`
}

func (p *jsonWriter) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return nil
	}
}

func (p *jsonWriter) Event(e Event) {
	var message = &jsonMessage{
		DryRun: p.dryRun,
		Tail:   false,
		ID:     e.ID,
		Text:   e.Text,
		Status: e.StatusText,
	}
	marshal, err := json.Marshal(message)
	if err == nil {
		fmt.Fprintln(p.out, string(marshal))
	}
}

func (p *jsonWriter) Events(events []Event) {
	for _, e := range events {
		p.Event(e)
	}
}

func (p *jsonWriter) TailMsgf(msg string, args ...interface{}) {
	var message = &jsonMessage{
		DryRun: p.dryRun,
		Tail:   true,
		ID:     "",
		Text:   fmt.Sprintf(msg, args...),
		Status: "",
	}
	marshal, err := json.Marshal(message)
	if err == nil {
		fmt.Fprintln(p.out, string(marshal))
	}
}

func (p *jsonWriter) Stop() {
	p.done <- true
}

func (p *jsonWriter) HasMore(bool) {
}
