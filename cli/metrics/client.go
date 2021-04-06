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

package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"time"
)

type client struct {
	httpClient *http.Client
}

// Command is a command
type Command struct {
	Command string `json:"command"`
	Context string `json:"context"`
	Source  string `json:"source"`
	Status  string `json:"status"`
}

// CLISource is sent for cli metrics
var CLISource = "cli"

func init() {
	if v, ok := os.LookupEnv("DOCKER_METRICS_SOURCE"); ok {
		CLISource = v
	}
}

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
	result := make(chan bool, 1)
	go func() {
		postMetrics(command, c)
		result <- true
	}()

	// wait for the post finished, or timeout in case anything freezes.
	// Posting metrics without Desktop listening returns in less than a ms, and a handful of ms (often <2ms) when Desktop is listening
	select {
	case <-result:
	case <-time.After(50 * time.Millisecond):
	}
}

func postMetrics(command Command, c *client) {
	req, err := json.Marshal(command)
	if err == nil {
		_, _ = c.httpClient.Post("http://localhost/usage", "application/json", bytes.NewBuffer(req))
	}
}
