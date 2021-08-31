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
	"encoding/json"
	"fmt"

	"github.com/docker/compose/v2/pkg/api"

	"github.com/spf13/cobra"
)

type eventsOpts struct {
	*composeOptions
	json bool
}

func eventsCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := eventsOpts{
		composeOptions: &composeOptions{
			projectOptions: p,
		},
	}
	cmd := &cobra.Command{
		Use:   "events [options] [--] [SERVICE...]",
		Short: "Receive real time events from containers.",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runEvents(ctx, backend, opts, args)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}

	cmd.Flags().BoolVar(&opts.json, "json", false, "Output events as a stream of json objects")
	return cmd
}

func runEvents(ctx context.Context, backend api.Service, opts eventsOpts, services []string) error {
	project, err := opts.toProjectName()
	if err != nil {
		return err
	}

	return backend.Events(ctx, project, api.EventsOptions{
		Services: services,
		Consumer: func(event api.Event) error {
			if opts.json {
				marshal, err := json.Marshal(map[string]interface{}{
					"time":       event.Timestamp,
					"type":       "container",
					"service":    event.Service,
					"id":         event.Container,
					"action":     event.Status,
					"attributes": event.Attributes,
				})
				if err != nil {
					return err
				}
				fmt.Println(string(marshal))
			} else {
				fmt.Println(event)
			}
			return nil
		},
	})
}
