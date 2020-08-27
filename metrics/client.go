/*
   Copyright 2020 Docker, Inc.

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

package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
)

type client struct {
	httpClient *http.Client
}

// Command is a command
type Command struct {
	Command string `json:"command"`
	Context string `json:"context"`
	Source  string `json:"source"`
}

const (
	// CLISource is sent for cli metrics
	CLISource = "cli"
	// APISource is sent for API metrics
	APISource = "api"
)

// Client sends metrics to Docker Desktopn
type Client interface {
	// Send sends the command to Docker Desktop. Note that the function doesn't
	// return anything, not even an error, this is because we don't really care
	// if the metrics were sent or not. We only fire and forget.
	Send(Command)
}

// NewClient returns a new metrics client
func NewClient() Client {
	return &client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return conn()
				},
			},
		},
	}
}

func (c *client) Send(command Command) {
	req, err := json.Marshal(command)
	if err != nil {
		return
	}

	_, _ = c.httpClient.Post("http://localhost/usage", "application/json", bytes.NewBuffer(req))
}
