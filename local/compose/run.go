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
	"io"
	"os"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose-cli/api/compose"
	convert "github.com/docker/compose-cli/local/moby"
	apitypes "github.com/docker/docker/api/types"
	moby "github.com/docker/docker/pkg/stringid"
)

func (s *composeService) RunOneOffContainer(ctx context.Context, project *types.Project, opts compose.RunOptions) (string, error) {
	originalServices := project.Services
	var requestedService types.ServiceConfig
	for _, service := range originalServices {
		if service.Name == opts.Name {
			requestedService = service
		}
	}

	project.Services = originalServices
	if len(opts.Command) > 0 {
		requestedService.Command = opts.Command
	}
	requestedService.Scale = 1
	requestedService.Tty = true
	requestedService.StdinOpen = true

	slug := moby.GenerateRandomID()
	requestedService.ContainerName = fmt.Sprintf("%s_%s_run_%s", project.Name, requestedService.Name, moby.TruncateID(slug))
	requestedService.Labels = requestedService.Labels.Add(slugLabel, slug)
	requestedService.Labels = requestedService.Labels.Add(oneoffLabel, "True")

	if err := s.waitDependencies(ctx, project, requestedService); err != nil {
		return "", err
	}
	err := s.createContainer(ctx, project, requestedService, requestedService.ContainerName, 1)
	if err != nil {
		return "", err
	}

	containerID := requestedService.ContainerName

	if opts.Detach {
		return containerID, s.apiClient.ContainerStart(ctx, containerID, apitypes.ContainerStartOptions{})
	}

	cnx, err := s.apiClient.ContainerAttach(ctx, containerID, apitypes.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
		Logs:   true,
	})
	if err != nil {
		return containerID, err
	}
	defer cnx.Close()

	stdout := convert.ContainerStdout{HijackedResponse: cnx}
	stdin := convert.ContainerStdin{HijackedResponse: cnx}

	readChannel := make(chan error, 10)
	writeChannel := make(chan error, 10)

	go func() {
		_, err := io.Copy(os.Stdout, cnx.Reader)
		readChannel <- err
	}()

	go func() {
		_, err := io.Copy(stdin, os.Stdin)
		writeChannel <- err
	}()

	go func() {
		<-ctx.Done()
		stdout.Close() //nolint:errcheck
		stdin.Close()  //nolint:errcheck
	}()

	// start container
	err = s.apiClient.ContainerStart(ctx, containerID, apitypes.ContainerStartOptions{})
	if err != nil {
		return containerID, err
	}

	for {
		select {
		case err := <-readChannel:
			return containerID, err
		case err := <-writeChannel:
			return containerID, err
		}
	}
}
