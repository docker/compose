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

	"github.com/compose-spec/compose-go/types"
	"github.com/distribution/reference"
	"github.com/docker/compose/v2/pkg/api"
)

func (s *composeService) Publish(ctx context.Context, project *types.Project, repository string, options api.PublishOptions) error {
	err := s.Push(ctx, project, api.PushOptions{})
	if err != nil {
		return err
	}

	_, err = reference.ParseDockerRef(repository)
	if err != nil {
		return err
	}

	// TODO publish project.ComposeFiles

	return api.ErrNotImplemented
}
