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
	"context"
	"strings"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
)

func (s *composeService) Events(ctx context.Context, projectName string, options api.EventsOptions) error {
	projectName = strings.ToLower(projectName)
	evts, errors := s.apiClient().Events(ctx, events.ListOptions{
		Filters: filters.NewArgs(projectFilter(projectName)),
	})
	for {
		select {
		case event := <-evts:
			// TODO: support other event types
			if event.Type != "container" {
				continue
			}

			oneOff := event.Actor.Attributes[api.OneoffLabel]
			if oneOff == "True" {
				// ignore
				continue
			}
			service := event.Actor.Attributes[api.ServiceLabel]
			if len(options.Services) > 0 && !utils.StringContains(options.Services, service) {
				continue
			}

			attributes := map[string]string{}
			for k, v := range event.Actor.Attributes {
				if strings.HasPrefix(k, "com.docker.compose.") {
					continue
				}
				attributes[k] = v
			}

			timestamp := time.Unix(event.Time, 0)
			if event.TimeNano != 0 {
				timestamp = time.Unix(0, event.TimeNano)
			}
			err := options.Consumer(api.Event{
				Timestamp:  timestamp,
				Service:    service,
				Container:  event.Actor.ID,
				Status:     string(event.Action),
				Attributes: attributes,
			})
			if err != nil {
				return err
			}

		case err := <-errors:
			return err
		}
	}
}
