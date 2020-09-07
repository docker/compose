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

package client

import (
	"context"
	"github.com/docker/compose-cli/api/volumes"
	"github.com/docker/compose-cli/errdefs"
)

type volumeService struct {
}

// List list volumes
func (c *volumeService) List(ctx context.Context) ([]volumes.Volume, error) {
	return nil, errdefs.ErrNotImplemented
}

// Create creates a volume
func (c *volumeService) Create(ctx context.Context, options interface {}) (volumes.Volume, error) {
	return volumes.Volume{}, errdefs.ErrNotImplemented
}
