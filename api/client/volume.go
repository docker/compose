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

package client

import (
	"context"

	"github.com/docker/compose-cli/api/volumes"
	"github.com/docker/compose-cli/errdefs"
)

type volumeService struct {
}

func (c *volumeService) List(ctx context.Context) ([]volumes.Volume, error) {
	return nil, errdefs.ErrNotImplemented
}

func (c *volumeService) Create(ctx context.Context, name string, options interface{}) (volumes.Volume, error) {
	return volumes.Volume{}, errdefs.ErrNotImplemented
}

func (c *volumeService) Delete(ctx context.Context, id string, options interface{}) error {
	return errdefs.ErrNotImplemented
}

func (c *volumeService) Inspect(ctx context.Context, volumeID string) (volumes.Volume, error) {
	return volumes.Volume{}, errdefs.ErrNotImplemented
}
