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

	"github.com/docker/compose/v2/pkg/api"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Wait(ctx context.Context, projectName string, options api.WaitOptions) (int64, error) {
	containers, err := s.getContainers(ctx, projectName, oneOffInclude, false, options.Services...)
	if err != nil {
		return 0, err
	}
	if len(containers) == 0 {
		return 0, fmt.Errorf("no containers for project %q", projectName)
	}

	eg, waitCtx := errgroup.WithContext(ctx)
	var statusCode int64
	for _, c := range containers {
		c := c
		eg.Go(func() error {
			var err error
			resultC, errC := s.dockerCli.Client().ContainerWait(waitCtx, c.ID, "")

			select {
			case result := <-resultC:
				_, _ = fmt.Fprintf(s.dockerCli.Out(), "container %q exited with status code %d\n", c.ID, result.StatusCode)
				statusCode = result.StatusCode
			case err = <-errC:
			}

			return err
		})
	}

	err = eg.Wait()
	if err != nil {
		return 42, err // Ignore abort flag in case of error in wait
	}

	if options.DownProjectOnContainerExit {
		return statusCode, s.Down(ctx, projectName, api.DownOptions{
			RemoveOrphans: true,
		})
	}

	return statusCode, err
}
