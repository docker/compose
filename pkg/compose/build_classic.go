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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/cli/cli/command"

	"github.com/docker/docker/api/types/registry"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command/image/build"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/builder/remotecontext/urlutil"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/idtools"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/progress"
	"github.com/docker/docker/pkg/streamformatter"

	"github.com/docker/compose/v2/pkg/api"
)

//nolint:gocyclo
func (s *composeService) doBuildClassic(ctx context.Context, project *types.Project, service types.ServiceConfig, options api.BuildOptions) (string, error) {
	var (
		buildCtx      io.ReadCloser
		dockerfileCtx io.ReadCloser
		contextDir    string
		tempDir       string
		relDockerfile string

		err error
	)

	dockerfileName := dockerFilePath(service.Build.Context, service.Build.Dockerfile)
	specifiedContext := service.Build.Context
	progBuff := s.stdout()
	buildBuff := s.stdout()

	if len(service.Build.Platforms) > 1 {
		return "", fmt.Errorf("the classic builder doesn't support multi-arch build, set DOCKER_BUILDKIT=1 to use BuildKit")
	}
	if service.Build.Privileged {
		return "", fmt.Errorf("the classic builder doesn't support privileged mode, set DOCKER_BUILDKIT=1 to use BuildKit")
	}
	if len(service.Build.AdditionalContexts) > 0 {
		return "", fmt.Errorf("the classic builder doesn't support additional contexts, set DOCKER_BUILDKIT=1 to use BuildKit")
	}
	if len(service.Build.SSH) > 0 {
		return "", fmt.Errorf("the classic builder doesn't support SSH keys, set DOCKER_BUILDKIT=1 to use BuildKit")
	}
	if len(service.Build.Secrets) > 0 {
		return "", fmt.Errorf("the classic builder doesn't support secrets, set DOCKER_BUILDKIT=1 to use BuildKit")
	}

	if service.Build.Labels == nil {
		service.Build.Labels = make(map[string]string)
	}
	service.Build.Labels[api.ImageBuilderLabel] = "classic"

	switch {
	case isLocalDir(specifiedContext):
		contextDir, relDockerfile, err = build.GetContextFromLocalDir(specifiedContext, dockerfileName)
		if err == nil && strings.HasPrefix(relDockerfile, ".."+string(filepath.Separator)) {
			// Dockerfile is outside of build-context; read the Dockerfile and pass it as dockerfileCtx
			dockerfileCtx, err = os.Open(dockerfileName)
			if err != nil {
				return "", fmt.Errorf("unable to open Dockerfile: %w", err)
			}
			defer dockerfileCtx.Close() //nolint:errcheck
		}
	case urlutil.IsGitURL(specifiedContext):
		tempDir, relDockerfile, err = build.GetContextFromGitURL(specifiedContext, dockerfileName)
	case urlutil.IsURL(specifiedContext):
		buildCtx, relDockerfile, err = build.GetContextFromURL(progBuff, specifiedContext, dockerfileName)
	default:
		return "", fmt.Errorf("unable to prepare context: path %q not found", specifiedContext)
	}

	if err != nil {
		return "", fmt.Errorf("unable to prepare context: %w", err)
	}

	if tempDir != "" {
		defer os.RemoveAll(tempDir) //nolint:errcheck
		contextDir = tempDir
	}

	// read from a directory into tar archive
	if buildCtx == nil {
		excludes, err := build.ReadDockerignore(contextDir)
		if err != nil {
			return "", err
		}

		if err := build.ValidateContextDirectory(contextDir, excludes); err != nil {
			return "", fmt.Errorf("checking context: %w", err)
		}

		// And canonicalize dockerfile name to a platform-independent one
		relDockerfile = filepath.ToSlash(relDockerfile)

		excludes = build.TrimBuildFilesFromExcludes(excludes, relDockerfile, false)
		buildCtx, err = archive.TarWithOptions(contextDir, &archive.TarOptions{
			ExcludePatterns: excludes,
			ChownOpts:       &idtools.Identity{},
		})
		if err != nil {
			return "", err
		}
	}

	// replace Dockerfile if it was added from stdin or a file outside the build-context, and there is archive context
	if dockerfileCtx != nil && buildCtx != nil {
		buildCtx, relDockerfile, err = build.AddDockerfileToBuildContext(dockerfileCtx, buildCtx)
		if err != nil {
			return "", err
		}
	}

	buildCtx, err = build.Compress(buildCtx)
	if err != nil {
		return "", err
	}

	progressOutput := streamformatter.NewProgressOutput(progBuff)
	body := progress.NewProgressReader(buildCtx, progressOutput, 0, "", "Sending build context to Docker daemon")

	configFile := s.configFile()
	creds, err := configFile.GetAllCredentials()
	if err != nil {
		return "", err
	}
	authConfigs := make(map[string]registry.AuthConfig, len(creds))
	for k, auth := range creds {
		authConfigs[k] = registry.AuthConfig(auth)
	}
	buildOptions := imageBuildOptions(s.dockerCli, project, service, options)
	imageName := api.GetImageNameOrDefault(service, project.Name)
	buildOptions.Tags = append(buildOptions.Tags, imageName)
	buildOptions.Dockerfile = relDockerfile
	buildOptions.AuthConfigs = authConfigs
	buildOptions.Memory = options.Memory

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	response, err := s.apiClient().ImageBuild(ctx, body, buildOptions)
	if err != nil {
		return "", err
	}
	defer response.Body.Close() //nolint:errcheck

	imageID := ""
	aux := func(msg jsonmessage.JSONMessage) {
		var result dockertypes.BuildResult
		if err := json.Unmarshal(*msg.Aux, &result); err != nil {
			fmt.Fprintf(s.stderr(), "Failed to parse aux message: %s", err)
		} else {
			imageID = result.ID
		}
	}

	err = jsonmessage.DisplayJSONMessagesStream(response.Body, buildBuff, progBuff.FD(), true, aux)
	if err != nil {
		var jerr *jsonmessage.JSONError
		if errors.As(err, &jerr) {
			// If no error code is set, default to 1
			if jerr.Code == 0 {
				jerr.Code = 1
			}
			return "", cli.StatusError{Status: jerr.Message, StatusCode: jerr.Code}
		}
		return "", err
	}

	// Windows: show error message about modified file permissions if the
	// daemon isn't running Windows.
	if response.OSType != "windows" && runtime.GOOS == "windows" {
		// if response.OSType != "windows" && runtime.GOOS == "windows" && !options.quiet {
		fmt.Fprintln(s.stdout(), "SECURITY WARNING: You are building a Docker "+
			"image from Windows against a non-Windows Docker host. All files and "+
			"directories added to build context will have '-rwxr-xr-x' permissions. "+
			"It is recommended to double check and reset permissions for sensitive "+
			"files and directories.")
	}

	return imageID, nil
}

func isLocalDir(c string) bool {
	_, err := os.Stat(c)
	return err == nil
}

func imageBuildOptions(dockerCli command.Cli, project *types.Project, service types.ServiceConfig, options api.BuildOptions) dockertypes.ImageBuildOptions {
	config := service.Build
	return dockertypes.ImageBuildOptions{
		Version:     dockertypes.BuilderV1,
		Tags:        config.Tags,
		NoCache:     config.NoCache,
		Remove:      true,
		PullParent:  config.Pull,
		BuildArgs:   resolveAndMergeBuildArgs(dockerCli, project, service, options),
		Labels:      config.Labels,
		NetworkMode: config.Network,
		ExtraHosts:  config.ExtraHosts.AsList(":"),
		Target:      config.Target,
		Isolation:   container.Isolation(config.Isolation),
	}
}
