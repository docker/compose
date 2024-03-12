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

package compose

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/compose/v2/internal/desktop"
	"github.com/sirupsen/logrus"
)

// engineLabelDesktopAddress is used to detect that Compose is running with a
// Docker Desktop context. When this label is present, the value is an endpoint
// address for an in-memory socket (AF_UNIX or named pipe).
const engineLabelDesktopAddress = "com.docker.desktop.address"

var _ desktop.IntegrationService = &composeService{}

// MaybeEnableDesktopIntegration initializes the desktop.Client instance if
// the server info from the Docker Engine is a Docker Desktop instance.
//
// EXPERIMENTAL: Requires `COMPOSE_EXPERIMENTAL_DESKTOP=1` env var set.
func (s *composeService) MaybeEnableDesktopIntegration(ctx context.Context) error {
	if desktopEnabled, _ := strconv.ParseBool(os.Getenv("COMPOSE_EXPERIMENTAL_DESKTOP")); !desktopEnabled {
		return nil
	}

	if s.dryRun {
		return nil
	}

	// safeguard to make sure this doesn't get stuck indefinitely
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	info, err := s.dockerCli.Client().Info(ctx)
	if err != nil {
		return fmt.Errorf("querying server info: %w", err)
	}
	for _, l := range info.Labels {
		k, v, ok := strings.Cut(l, "=")
		if !ok || k != engineLabelDesktopAddress {
			continue
		}

		desktopCli := desktop.NewClient(v)
		_, err := desktopCli.Ping(ctx)
		if err != nil {
			return fmt.Errorf("pinging Desktop API: %w", err)
		}
		logrus.Debugf("Enabling Docker Desktop integration (experimental): %s", v)
		s.desktopCli = desktopCli
		return nil
	}

	logrus.Trace("Docker Desktop not detected, no integration enabled")
	return nil
}
