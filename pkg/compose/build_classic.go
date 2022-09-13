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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/compose-spec/compose-go/types"
	buildx "github.com/docker/buildx/build"
	"github.com/docker/cli/cli/command/image/build"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/cli"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/idtools"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/progress"
	"github.com/docker/docker/pkg/streamformatter"
	"github.com/docker/docker/pkg/urlutil"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/docker/compose/v2/pkg/api"
)

func (s *composeService) doBuildClassic(ctx context.Context, project *types.Project, opts map[string]buildx.Options) (map[string]string, error) {
	var nameDigests = make(map[string]string)
	var errs error
	err := project.WithServices(nil, func(service types.ServiceConfig) error {
		imageName := api.GetImageNameOrDefault(service, project.Name)
		o, ok := opts[imageName]
		if !ok {
			return nil
		}
		digest, err := s.doBuildClassicSimpleImage(ctx, o)
		if err != nil {
			errs = multierror.Append(errs, err).ErrorOrNil()
		}
		nameDigests[imageName] = digest
		return nil
	})
	if err != nil {
		return nil, err
	}

	return nameDigests, errs
}

//nolint:gocyclo
func (s *composeService) doBuildClassicSimpleImage(ctx context.Context, options buildx.Options) (string, error) {
	var (
		buildCtx      io.ReadCloser
		dockerfileCtx io.ReadCloser
		contextDir    string
		tempDir       string
		relDockerfile string

		err error
	)

	dockerfileName := options.Inputs.DockerfilePath
	specifiedContext := options.Inputs.ContextPath
	progBuff := s.stdout()
	buildBuff := s.stdout()
	if options.ImageIDFile != "" {
		// Avoid leaving a stale file if we eventually fail
		if err := os.Remove(options.ImageIDFile); err != nil && !os.IsNotExist(err) {
			return "", errors.Wrap(err, "removing image ID file")
		}
	}

	if len(options.Platforms) > 1 {
		return "", errors.Errorf("this builder doesn't support multi-arch build, set DOCKER_BUILDKIT=1 to use multi-arch builder")
	}

	if options.Labels == nil {
		options.Labels = make(map[string]string)
	}
	options.Labels[api.ImageBuilderLabel] = "classic"

	switch {
	case isLocalDir(specifiedContext):
		contextDir, relDockerfile, err = build.GetContextFromLocalDir(specifiedContext, dockerfileName)
		if err == nil && strings.HasPrefix(relDockerfile, ".."+string(filepath.Separator)) {
			// Dockerfile is outside of build-context; read the Dockerfile and pass it as dockerfileCtx
			dockerfileCtx, err = os.Open(dockerfileName)
			if err != nil {
				return "", errors.Errorf("unable to open Dockerfile: %v", err)
			}
			defer dockerfileCtx.Close() //nolint:errcheck
		}
	case urlutil.IsGitURL(specifiedContext):
		tempDir, relDockerfile, err = build.GetContextFromGitURL(specifiedContext, dockerfileName)
	case urlutil.IsURL(specifiedContext):
		buildCtx, relDockerfile, err = build.GetContextFromURL(progBuff, specifiedContext, dockerfileName)
	default:
		return "", errors.Errorf("unable to prepare context: path %q not found", specifiedContext)
	}

	if err != nil {
		return "", errors.Errorf("unable to prepare context: %s", err)
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
			return "", errors.Wrap(err, "checking context")
		}

		// And canonicalize dockerfile name to a platform-independent one
		relDockerfile = archive.CanonicalTarNameForPath(relDockerfile)

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
	authConfigs := make(map[string]dockertypes.AuthConfig, len(creds))
	for k, auth := range creds {
		authConfigs[k] = dockertypes.AuthConfig(auth)
	}
	buildOptions := imageBuildOptions(options)
	buildOptions.Version = dockertypes.BuilderV1
	buildOptions.Dockerfile = relDockerfile
	buildOptions.AuthConfigs = authConfigs

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
		if jerr, ok := err.(*jsonmessage.JSONError); ok {
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

	if options.ImageIDFile != "" {
		if imageID == "" {
			return "", errors.Errorf("Server did not provide an image ID. Cannot write %s", options.ImageIDFile)
		}
		if err := os.WriteFile(options.ImageIDFile, []byte(imageID), 0o666); err != nil {
			return "", err
		}
	}

	return imageID, nil
}

func isLocalDir(c string) bool {
	_, err := os.Stat(c)
	return err == nil
}

func imageBuildOptions(options buildx.Options) dockertypes.ImageBuildOptions {
	return dockertypes.ImageBuildOptions{
		Tags:        options.Tags,
		NoCache:     options.NoCache,
		Remove:      true,
		PullParent:  options.Pull,
		BuildArgs:   toMapStringStringPtr(options.BuildArgs),
		Labels:      options.Labels,
		NetworkMode: options.NetworkMode,
		ExtraHosts:  options.ExtraHosts,
		Target:      options.Target,
	}
}

func toMapStringStringPtr(source map[string]string) map[string]*string {
	dest := make(map[string]*string)
	for k, v := range source {
		v := v
		dest[k] = &v
	}
	return dest
}
