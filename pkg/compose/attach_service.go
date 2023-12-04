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

	"github.com/docker/cli/cli/command/container"
	"github.com/docker/compose/v2/pkg/api"
)

func (s *composeService) Attach(ctx context.Context, projectName string, options api.AttachOptions) error {
	projectName = strings.ToLower(projectName)
	target, err := s.getSpecifiedContainer(ctx, projectName, oneOffInclude, false, options.Service, options.Index)
	if err != nil {
		return err
	}

	var attach container.AttachOptions
	attach.DetachKeys = options.DetachKeys
	attach.NoStdin = options.NoStdin
	attach.Proxy = options.Proxy
	return container.RunAttach(ctx, s.dockerCli, target.ID, &attach)
}
