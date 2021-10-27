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

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/pkg/api"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/docker/pkg/stringid"
	"github.com/moby/term"
)

func (s *composeService) RunOneOffContainer(ctx context.Context, project *types.Project, opts api.RunOptions) (int, error) {
	containerID, err := s.prepareRun(ctx, project, opts)
	if err != nil {
		return 0, err
	}

	if opts.Detach {
		err := s.apiClient.ContainerStart(ctx, containerID, moby.ContainerStartOptions{})
		if err != nil {
			return 0, err
		}
		fmt.Fprintln(opts.Stdout, containerID)
		return 0, nil
	}

	return s.runInteractive(ctx, containerID, opts)
}

func (s *composeService) runInteractive(ctx context.Context, containerID string, opts api.RunOptions) (int, error) {
	r, err := s.getEscapeKeyProxy(opts.Stdin, opts.Tty)
	if err != nil {
		return 0, err
	}

	stdin, stdout, err := s.getContainerStreams(ctx, containerID)
	if err != nil {
		return 0, err
	}

	in := streams.NewIn(opts.Stdin)
	if in.IsTerminal() {
		state, err := term.SetRawTerminal(in.FD())
		if err != nil {
			return 0, err
		}
		defer term.RestoreTerminal(in.FD(), state) //nolint:errcheck
	}

	outputDone := make(chan error)
	inputDone := make(chan error)

	go func() {
		if opts.Tty {
			_, err := io.Copy(opts.Stdout, stdout) //nolint:errcheck
			outputDone <- err
		} else {
			_, err := stdcopy.StdCopy(opts.Stdout, opts.Stderr, stdout) //nolint:errcheck
			outputDone <- err
		}
		stdout.Close() //nolint:errcheck
	}()

	go func() {
		_, err := io.Copy(stdin, r)
		inputDone <- err
		stdin.Close() //nolint:errcheck
	}()

	err = s.apiClient.ContainerStart(ctx, containerID, moby.ContainerStartOptions{})
	if err != nil {
		return 0, err
	}

	s.monitorTTySize(ctx, containerID, s.apiClient.ContainerResize)

	for {
		select {
		case err := <-outputDone:
			if err != nil {
				return 0, err
			}
			return s.terminateRun(ctx, containerID, opts)
		case err := <-inputDone:
			if _, ok := err.(term.EscapeError); ok {
				return 0, nil
			}
			if err != nil {
				return 0, err
			}
			// Wait for output to complete streaming
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}

func (s *composeService) terminateRun(ctx context.Context, containerID string, opts api.RunOptions) (exitCode int, err error) {
	exitCh, errCh := s.apiClient.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case exit := <-exitCh:
		exitCode = int(exit.StatusCode)
	case err = <-errCh:
		return
	}
	if opts.AutoRemove {
		err = s.apiClient.ContainerRemove(ctx, containerID, moby.ContainerRemoveOptions{})
	}
	return
}

func (s *composeService) prepareRun(ctx context.Context, project *types.Project, opts api.RunOptions) (string, error) {
	if err := prepareVolumes(project); err != nil { // all dependencies already checked, but might miss service img
		return "", err
	}
	service, err := project.GetService(opts.Service)
	if err != nil {
		return "", err
	}

	applyRunOptions(project, &service, opts)

	slug := stringid.GenerateRandomID()
	if service.ContainerName == "" {
		service.ContainerName = fmt.Sprintf("%s_%s_run_%s", project.Name, service.Name, stringid.TruncateID(slug))
	}
	service.Scale = 1
	service.StdinOpen = true
	service.Restart = ""
	if service.Deploy != nil {
		service.Deploy.RestartPolicy = nil
	}
	service.Labels = service.Labels.Add(api.SlugLabel, slug)
	service.Labels = service.Labels.Add(api.OneoffLabel, "True")

	if err := s.ensureImagesExists(ctx, project, false); err != nil { // all dependencies already checked, but might miss service img
		return "", err
	}
	if !opts.NoDeps {
		if err := s.waitDependencies(ctx, project, service); err != nil {
			return "", err
		}
	}

	observedState, err := s.getContainers(ctx, project.Name, oneOffInclude, true)
	if err != nil {
		return "", err
	}
	updateServices(&service, observedState)

	created, err := s.createContainer(ctx, project, service, service.ContainerName, 1, opts.Detach && opts.AutoRemove, opts.UseNetworkAliases, true)
	if err != nil {
		return "", err
	}
	containerID := created.ID
	return containerID, nil
}

func (s *composeService) getEscapeKeyProxy(r io.ReadCloser, isTty bool) (io.ReadCloser, error) {
	if !isTty {
		return r, nil
	}
	var escapeKeys = []byte{16, 17}
	if s.configFile.DetachKeys != "" {
		customEscapeKeys, err := term.ToBytes(s.configFile.DetachKeys)
		if err != nil {
			return nil, err
		}
		escapeKeys = customEscapeKeys
	}
	return ioutils.NewReadCloserWrapper(term.NewEscapeProxy(r, escapeKeys), r.Close), nil
}

func applyRunOptions(project *types.Project, service *types.ServiceConfig, opts api.RunOptions) {
	service.Tty = opts.Tty
	service.StdinOpen = true
	service.ContainerName = opts.Name

	if len(opts.Command) > 0 {
		service.Command = opts.Command
	}
	if len(opts.User) > 0 {
		service.User = opts.User
	}
	if len(opts.WorkingDir) > 0 {
		service.WorkingDir = opts.WorkingDir
	}
	if opts.Entrypoint != nil {
		service.Entrypoint = opts.Entrypoint
	}
	if len(opts.Environment) > 0 {
		env := types.NewMappingWithEquals(opts.Environment)
		projectEnv := env.Resolve(func(s string) (string, bool) {
			v, ok := project.Environment[s]
			return v, ok
		}).RemoveEmpty()
		service.Environment.OverrideBy(projectEnv)
	}
	for k, v := range opts.Labels {
		service.Labels = service.Labels.Add(k, v)
	}
}
