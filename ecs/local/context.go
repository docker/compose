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

package local

import (
	"context"

	"github.com/docker/compose-cli/context/cloud"
	"github.com/docker/compose-cli/ecs"
	"github.com/docker/compose-cli/errdefs"
)

var _ cloud.Service = ecsLocalSimulation{}

func (e ecsLocalSimulation) Login(ctx context.Context, params interface{}) error {
	return errdefs.ErrNotImplemented
}

func (e ecsLocalSimulation) Logout(ctx context.Context) error {
	return errdefs.ErrNotImplemented
}

func (e ecsLocalSimulation) CreateContextData(ctx context.Context, params interface{}) (contextData interface{}, description string, err error) {
	opts := params.(ecs.ContextParams)
	return struct{}{}, opts.Description, nil
}
