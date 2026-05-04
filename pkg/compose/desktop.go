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

	"github.com/docker/compose/v5/internal/desktop"
)

func (s *composeService) desktopEndpoint(ctx context.Context) (string, error) {
	return desktop.Endpoint(ctx, s.apiClient())
}

// isDesktopIntegrationActive returns true when Docker Desktop is the active engine.
func (s *composeService) isDesktopIntegrationActive(ctx context.Context) (bool, error) {
	endpoint, err := s.desktopEndpoint(ctx)
	return endpoint != "", err
}

// isDesktopFeatureActive checks whether a Docker Desktop feature is both
// available (feature flag) and enabled by the user (settings). Returns false
// silently when Desktop is not running or unreachable.
func (s *composeService) isDesktopFeatureActive(ctx context.Context, feature string) bool {
	endpoint, err := s.desktopEndpoint(ctx)
	if err != nil || endpoint == "" {
		return false
	}

	ddClient := desktop.NewClient(endpoint)
	defer ddClient.Close() //nolint:errcheck

	enabled, err := ddClient.IsFeatureEnabled(ctx, feature)
	if err != nil {
		return false
	}
	return enabled
}
