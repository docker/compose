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
	"fmt"

	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/api"
)

type progressFunc func(context.Context) error

func Run(ctx context.Context, pf progressFunc, operation string, bus api.EventProcessor) error {
	bus.Start(ctx, operation)
	err := pf(ctx)
	bus.Done(operation, err != nil)
	return err
}

// errorEventf creates a new Error Resource with format message
func errorEventf(id string, msg string, args ...any) api.Resource {
	return progress.Error(id, fmt.Sprintf(msg, args...))
}

// buildingEvent creates a new Building in progress Resource
func buildingEvent(id string) api.Resource {
	return progress.New("Image "+id, api.Working, api.StatusBuilding)
}

// builtEvent creates a new built (done) Resource
func builtEvent(id string) api.Resource {
	return progress.New("Image "+id, api.Done, api.StatusBuilt)
}

type ignore struct{}

func (q *ignore) Start(_ context.Context, _ string) {
}

func (q *ignore) Done(_ string, _ bool) {
}

func (q *ignore) On(_ ...api.Resource) {
}
