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

package desktop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/cli/cli/command"
)

// engineLabelDesktopAddress is used to detect that Compose is running with a
// Docker Desktop context. When this label is present, the value is an endpoint
// address for an in-memory socket (AF_UNIX or named pipe).
const engineLabelDesktopAddress = "com.docker.desktop.address"

// NewFromDockerClient creates a Desktop Client using the Docker CLI client to
// auto-discover the Desktop CLI socket endpoint (if available).
//
// An error is returned if there is a failure communicating with Docker Desktop,
// but even on success, a nil Client can be returned if the active Docker Engine
// is not a Desktop instance.
func NewFromDockerClient(ctx context.Context, dockerCli command.Cli) (*Client, error) {
	// safeguard to make sure this doesn't get stuck indefinitely
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	info, err := dockerCli.Client().Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying server info: %w", err)
	}
	for _, l := range info.Labels {
		k, v, ok := strings.Cut(l, "=")
		if !ok || k != engineLabelDesktopAddress {
			continue
		}

		desktopCli := NewClient(v)
		_, err := desktopCli.Ping(ctx)
		if err != nil {
			return nil, fmt.Errorf("pinging Desktop API: %w", err)
		}
		return desktopCli, nil
	}
	return nil, nil
}
