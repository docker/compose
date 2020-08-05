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

package compose

import (
	"context"

	"github.com/compose-spec/compose-go/cli"
	types "github.com/docker/ecs-plugin/pkg/compose"
)

// Service manages a compose project
type Service interface {
	// Up executes the equivalent to a `compose up`
	Up(ctx context.Context, opts cli.ProjectOptions) error
	// Down executes the equivalent to a `compose down`
	Down(ctx context.Context, opts cli.ProjectOptions) error
	Logs(ctx context.Context, projectName cli.ProjectOptions) error
	Ps(background context.Context, options cli.ProjectOptions) ([]types.ServiceStatus, error)
}
