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
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command/image/build"
	"github.com/moby/go-archive"
	buildtypes "github.com/moby/moby/api/types/build"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/jsonmessage"
	"github.com/moby/moby/client/pkg/progress"
	"github.com/moby/moby/client/pkg/streamformatter"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) doBuildClassic(ctx context.Context, project *types.Project, serviceToBuild types.Services, options api.BuildOptions) (map[string]string, error) {
	imageIDs := map[string]string{}

	// Not using bake, additional_context: service:xx is implemented by building images in dependency order
	project, err := project.WithServicesTransform(func(serviceName string, service types.ServiceConfig) (types.ServiceConfig, error) {
		if service.Build != nil {
			for _, additionalContext := range service.Build.AdditionalContexts {
				if targetService, found := strings.CutPrefix(additionalContext, types.ServicePrefix); found {
					if service.DependsOn == nil {
						service.DependsOn = map[string]types.ServiceDependency{}
					}
					service.DependsOn[targetService] = types.ServiceDependency{
						Condition: "build", // non-canonical, but will force dependency graph ordering
					}
				}
			}
		}
		return service, nil
	})
	if err != nil {
		return imageIDs, err
	}

	// we use a pre-allocated []string to collect build digest by service index while running concurrent goroutines
	builtDigests := make([]string, len(project.Services))
	names := project.ServiceNames()
	getServiceIndex := func(name string) int {
		for idx, n := range names {
			if n == name {
				return idx
			}
		}
		return -1
	}

	err = InDependencyOrder(ctx, project, func(ctx context.Context, name string) error {
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("builder", "classic"))
		service, ok := serviceToBuild[name]
		if !ok {
			return nil
		}

		image := api.GetImageNameOrDefault(service, project.Name)
		s.events.On(buildingEvent(image))
		id, err := s.doBuildImage(ctx, project, service, options)
		if err != nil {
			return err
		}
		s.events.On(builtEvent(image))
		// the classic builder reports the raw image ID from the build stream;
		// resolve the canonical content digest instead so the recorded
		// identity matches what later runs compute for the same local image
		// (resolved here to inherit the build traversal's concurrency)
		builtDigests[getServiceIndex(name)] = s.canonicalBuiltDigest(ctx, image, service.Platform, id)

		if options.Push {
			return s.push(ctx, project, api.PushOptions{})
		}
		return nil
	}, func(traversal *graphTraversal) {
		traversal.maxConcurrency = s.maxConcurrency
	})
	if err != nil {
		return nil, err
	}

	for i, imageDigest := range builtDigests {
		if imageDigest != "" {
			service := project.Services[names[i]]
			imageRef := api.GetImageNameOrDefault(service, project.Name)
			imageIDs[imageRef] = imageDigest
		}
	}
	return imageIDs, err
}

// classicBuildContext is the build context resolved for the classic builder:
// either an archive stream (buildCtx) or a local directory still to be tarred
// (contextDir), with an optional out-of-context Dockerfile stream. cleanup
// must run once the build is done.
type classicBuildContext struct {
	buildCtx      io.ReadCloser
	dockerfileCtx io.ReadCloser
	contextDir    string
	relDockerfile string
	cleanup       func()
}

func (s *composeService) doBuildImage(ctx context.Context, project *types.Project, service types.ServiceConfig, options api.BuildOptions) (string, error) {
	err := checkClassicBuilderSupported(service)
	if err != nil {
		return "", err
	}

	if service.Build.Labels == nil {
		service.Build.Labels = make(map[string]string)
	}
	service.Build.Labels[api.ImageBuilderLabel] = "classic"

	dockerfileName := dockerFilePath(service.Build.Context, service.Build.Dockerfile)
	specifiedContext := service.Build.Context
	progBuff := s.stdout()
	buildBuff := s.stdout()

	bctx, err := prepareClassicBuildContext(specifiedContext, dockerfileName, progBuff)
	if err != nil {
		return "", err
	}
	defer bctx.cleanup()

	buildCtx := bctx.buildCtx
	relDockerfile := bctx.relDockerfile
	// read from a directory into tar archive
	if buildCtx == nil {
		buildCtx, relDockerfile, err = archiveBuildContext(bctx.contextDir, relDockerfile)
		if err != nil {
			return "", err
		}
	}

	// replace Dockerfile if it was added from stdin or a file outside the build-context, and there is archive context
	if bctx.dockerfileCtx != nil && buildCtx != nil {
		buildCtx, relDockerfile, err = build.AddDockerfileToBuildContext(bctx.dockerfileCtx, buildCtx)
		if err != nil {
			return "", err
		}
	}

	buildCtx, err = build.Compress(buildCtx)
	if err != nil {
		return "", err
	}

	// Setup an upload progress bar
	progressOutput := streamformatter.NewProgressOutput(progBuff)
	body := progress.NewProgressReader(buildCtx, progressOutput, 0, "", "Sending build context to Docker daemon")

	authConfigs, err := s.classicAuthConfigs()
	if err != nil {
		return "", err
	}
	buildOpts := imageBuildOptions(s.getProxyConfig(), project, service, options)
	imageName := api.GetImageNameOrDefault(service, project.Name)
	buildOpts.Tags = append(buildOpts.Tags, imageName)
	buildOpts.Dockerfile = relDockerfile
	buildOpts.AuthConfigs = authConfigs
	buildOpts.Memory = options.Memory

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.events.On(buildingEvent(imageName))
	response, err := s.apiClient().ImageBuild(ctx, body, buildOpts)
	if err != nil {
		return "", err
	}
	defer response.Body.Close() //nolint:errcheck

	imageID := ""
	aux := func(msg jsonstream.Message) {
		var result buildtypes.Result
		if err := json.Unmarshal(*msg.Aux, &result); err != nil {
			logrus.Errorf("Failed to parse aux message: %s", err)
		} else {
			imageID = result.ID
		}
	}

	err = jsonmessage.DisplayJSONMessagesStream(response.Body, buildBuff, progBuff.FD(), true, aux)
	if err != nil {
		var jerr *jsonstream.Error
		if errors.As(err, &jerr) {
			// If no error code is set, default to 1
			if jerr.Code == 0 {
				jerr.Code = 1
			}
			return "", cli.StatusError{Status: jerr.Message, StatusCode: jerr.Code}
		}
		return "", err
	}
	s.events.On(builtEvent(imageName))
	return imageID, nil
}

// checkClassicBuilderSupported rejects build options the classic builder
// doesn't implement
func checkClassicBuilderSupported(service types.ServiceConfig) error {
	if len(service.Build.Platforms) > 1 {
		return fmt.Errorf("the classic builder doesn't support multi-arch build; building with BuildKit requires the buildx Docker CLI plugin, and DOCKER_BUILDKIT must not be set to 0")
	}
	if service.Build.Privileged {
		return fmt.Errorf("the classic builder doesn't support privileged mode; building with BuildKit requires the buildx Docker CLI plugin, and DOCKER_BUILDKIT must not be set to 0")
	}
	if len(service.Build.AdditionalContexts) > 0 {
		return fmt.Errorf("the classic builder doesn't support additional contexts; building with BuildKit requires the buildx Docker CLI plugin, and DOCKER_BUILDKIT must not be set to 0")
	}
	if len(service.Build.SSH) > 0 {
		return fmt.Errorf("the classic builder doesn't support SSH keys; building with BuildKit requires the buildx Docker CLI plugin, and DOCKER_BUILDKIT must not be set to 0")
	}
	if len(service.Build.Secrets) > 0 {
		return fmt.Errorf("the classic builder doesn't support secrets; building with BuildKit requires the buildx Docker CLI plugin, and DOCKER_BUILDKIT must not be set to 0")
	}
	return nil
}

// prepareClassicBuildContext resolves the build context per its type (local
// directory, git URL, remote URL)
func prepareClassicBuildContext(specifiedContext, dockerfileName string, progBuff io.Writer) (*classicBuildContext, error) {
	contextType, err := build.DetectContextType(specifiedContext)
	if err != nil {
		return nil, err
	}

	bctx := &classicBuildContext{cleanup: func() {}}
	switch contextType {
	case build.ContextTypeStdin:
		return nil, fmt.Errorf("building from STDIN is not supported")
	case build.ContextTypeLocal:
		bctx.contextDir, bctx.relDockerfile, err = build.GetContextFromLocalDir(specifiedContext, dockerfileName)
		if err != nil {
			return nil, fmt.Errorf("unable to prepare context: %w", err)
		}
		if strings.HasPrefix(bctx.relDockerfile, ".."+string(filepath.Separator)) {
			// Dockerfile is outside build-context; read the Dockerfile and pass it as dockerfileCtx
			dockerfileCtx, err := os.Open(dockerfileName)
			if err != nil {
				return nil, fmt.Errorf("unable to open Dockerfile: %w", err)
			}
			bctx.dockerfileCtx = dockerfileCtx
			bctx.cleanup = func() { _ = dockerfileCtx.Close() }
		}
	case build.ContextTypeGit:
		var tempDir string
		tempDir, bctx.relDockerfile, err = build.GetContextFromGitURL(specifiedContext, dockerfileName)
		if err != nil {
			return nil, fmt.Errorf("unable to prepare context: %w", err)
		}
		bctx.contextDir = tempDir
		bctx.cleanup = func() { _ = os.RemoveAll(tempDir) }
	case build.ContextTypeRemote:
		bctx.buildCtx, bctx.relDockerfile, err = build.GetContextFromURL(progBuff, specifiedContext, dockerfileName)
		if err != nil {
			return nil, fmt.Errorf("unable to prepare context: %w", err)
		}
	default:
		return nil, fmt.Errorf("unable to prepare context: path %q not found", specifiedContext)
	}
	return bctx, nil
}

// archiveBuildContext tars a local context directory, honoring .dockerignore
// and canonicalizing the dockerfile name to a platform-independent one
func archiveBuildContext(contextDir, relDockerfile string) (io.ReadCloser, string, error) {
	excludes, err := build.ReadDockerignore(contextDir)
	if err != nil {
		return nil, "", err
	}

	if err := build.ValidateContextDirectory(contextDir, excludes); err != nil {
		return nil, "", fmt.Errorf("checking context: %w", err)
	}

	relDockerfile = filepath.ToSlash(relDockerfile)

	excludes = build.TrimBuildFilesFromExcludes(excludes, relDockerfile, false)
	buildCtx, err := archive.TarWithOptions(contextDir, &archive.TarOptions{
		ExcludePatterns: excludes,
		ChownOpts:       &archive.ChownOpts{UID: 0, GID: 0},
	})
	if err != nil {
		return nil, "", err
	}
	return buildCtx, relDockerfile, nil
}

// classicAuthConfigs converts the CLI credentials to the engine's auth config
func (s *composeService) classicAuthConfigs() (map[string]registry.AuthConfig, error) {
	creds, err := s.configFile().GetAllCredentials()
	if err != nil {
		return nil, err
	}
	authConfigs := make(map[string]registry.AuthConfig, len(creds))
	for k, authConfig := range creds {
		authConfigs[k] = registry.AuthConfig{
			Username:      authConfig.Username,
			Password:      authConfig.Password,
			ServerAddress: authConfig.ServerAddress,

			// TODO(thaJeztah): Are these expected to be included? See https://github.com/docker/cli/pull/6516#discussion_r2387586472
			Auth:          authConfig.Auth,
			IdentityToken: authConfig.IdentityToken,
			RegistryToken: authConfig.RegistryToken,
		}
	}
	return authConfigs, nil
}

func imageBuildOptions(proxyConfigs map[string]string, project *types.Project, service types.ServiceConfig, options api.BuildOptions) client.ImageBuildOptions {
	config := service.Build
	return client.ImageBuildOptions{
		Version:     buildtypes.BuilderV1,
		Tags:        config.Tags,
		NoCache:     config.NoCache,
		Remove:      true,
		PullParent:  config.Pull,
		BuildArgs:   resolveAndMergeBuildArgs(proxyConfigs, project, service, options),
		Labels:      config.Labels,
		NetworkMode: config.Network,
		ExtraHosts:  config.ExtraHosts.AsList(":"),
		Target:      config.Target,
		Isolation:   container.Isolation(config.Isolation),
	}
}
