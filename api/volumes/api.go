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

package volumes

import (
	"context"
)

// Volume volume info
type Volume struct {
	ID          string
	Description string
}

// Service interacts with the underlying container backend
type Service interface {
	// List returns all available volumes
	List(ctx context.Context) ([]Volume, error)
	// Create creates a new volume
	Create(ctx context.Context, name string, options interface{}) (Volume, error)
	// Delete deletes an existing volume
	Delete(ctx context.Context, volumeID string, options interface{}) error
	// Inspect inspects an existing volume
	Inspect(ctx context.Context, volumeID string) (Volume, error)
}
