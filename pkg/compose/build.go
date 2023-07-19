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
	"os"
	"path/filepath"

	"github.com/docker/compose/v2/internal/tracing"

	"github.com/docker/buildx/controller/pb"

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/containerd/platforms"
	"github.com/docker/buildx/build"
	_ "github.com/docker/buildx/driver/docker" // required to get default driver registered
	"github.com/docker/buildx/store/storeutil"
	"github.com/docker/buildx/util/buildflags"
	xprogress "github.com/docker/buildx/util/progress"
	"github.com/docker/docker/builder/remotecontext/urlutil"
	bclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	"github.com/moby/buildkit/util/entitlements"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/utils"
)

func (s *composeService) Build(ctx context.Context, project *types.Project, options api.BuildOptions) error {
	err := options.Apply(project)
	if err != nil {
		return err
	}
	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		_, err := s.build(ctx, project, options)
		return err
	}, s.stdinfo(), "Building")
}

func (s *composeService) build(ctx context.Context, project *types.Project, options api.BuildOptions) (map[string]string, error) { //nolint:gocyclo
	args := options.Args.Resolve(envResolver(project.Environment))

	buildkitEnabled, err := s.dockerCli.BuildKitEnabled()
	if err != nil {
		return nil, err
	}

	// Progress needs its own context that lives longer than the
	// build one otherwise it won't read all the messages from
	// build and will lock
	progressCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := xprogress.NewPrinter(progressCtx, s.stdout(), os.Stdout, options.Progress)
	if err != nil {
		return nil, err
	}

	builtDigests := make([]string, len(project.Services))
	err = InDependencyOrder(ctx, project, func(ctx context.Context, name string) error {
		if len(options.Services) > 0 && !utils.Contains(options.Services, name) {
			return nil
		}
		service, idx := getServiceIndex(project, name)

		if service.Build == nil {
			return nil
		}

		if !buildkitEnabled {
			if service.Build.Args == nil {
				service.Build.Args = args
			} else {
				service.Build.Args = service.Build.Args.OverrideBy(args)
			}
			id, err := s.doBuildClassic(ctx, project.Name, service, options)
			if err != nil {
				return err
			}
			builtDigests[idx] = id

			if options.Push {
				return s.push(ctx, project, api.PushOptions{})
			}
			return nil
		}

		if options.Memory != 0 {
			fmt.Fprintln(s.stderr(), "WARNING: --memory is not supported by BuildKit and will be ignored.")
		}

		buildOptions, err := s.toBuildOptions(project, service, options)
		if err != nil {
			return err
		}
		buildOptions.BuildArgs = mergeArgs(buildOptions.BuildArgs, flatten(args))

		digest, err := s.doBuildBuildkit(ctx, service.Name, buildOptions, w, options.Builder)
		if err != nil {
			return err
		}
		builtDigests[idx] = digest

		return nil
	}, func(traversal *graphTraversal) {
		traversal.maxConcurrency = s.maxConcurrency
	})

	// enforce all build event get consumed
	if errw := w.Wait(); errw != nil {
		return nil, errw
	}

	if err != nil {
		return nil, err
	}

	imageIDs := map[string]string{}
	for i, imageDigest := range builtDigests {
		if imageDigest != "" {
			imageRef := api.GetImageNameOrDefault(project.Services[i], project.Name)
			imageIDs[imageRef] = imageDigest
		}
	}
	return imageIDs, err
}

func getServiceIndex(project *types.Project, name string) (types.ServiceConfig, int) {
	var service types.ServiceConfig
	var idx int
	for i, s := range project.Services {
		if s.Name == name {
			idx, service = i, s
			break
		}
	}
	return service, idx
}

func (s *composeService) ensureImagesExists(ctx context.Context, project *types.Project, quietPull bool) error {
	for _, service := range project.Services {
		if service.Image == "" && service.Build == nil {
			return fmt.Errorf("invalid service %q. Must specify either image or build", service.Name)
		}
	}

	images, err := s.getLocalImagesDigests(ctx, project)
	if err != nil {
		return err
	}

	err = tracing.SpanWrapFunc("project/pull", tracing.ProjectOptions(project),
		func(ctx context.Context) error {
			return s.pullRequiredImages(ctx, project, images, quietPull)
		},
	)(ctx)
	if err != nil {
		return err
	}

	mode := xprogress.PrinterModeAuto
	if quietPull {
		mode = xprogress.PrinterModeQuiet
	}

	buildRequired, err := s.prepareProjectForBuild(project, images)
	if err != nil {
		return err
	}

	if buildRequired {
		err = tracing.SpanWrapFunc("project/build", tracing.ProjectOptions(project),
			func(ctx context.Context) error {
				builtImages, err := s.build(ctx, project, api.BuildOptions{
					Progress: mode,
				})
				if err != nil {
					return err
				}

				for name, digest := range builtImages {
					images[name] = digest
				}
				return nil
			},
		)(ctx)
		if err != nil {
			return err
		}
	}

	// set digest as com.docker.compose.image label so we can detect outdated containers
	for i, service := range project.Services {
		image := api.GetImageNameOrDefault(service, project.Name)
		digest, ok := images[image]
		if ok {
			if project.Services[i].Labels == nil {
				project.Services[i].Labels = types.Labels{}
			}
			project.Services[i].CustomLabels.Add(api.ImageDigestLabel, digest)
		}
	}
	return nil
}

func (s *composeService) prepareProjectForBuild(project *types.Project, images map[string]string) (bool, error) {
	buildRequired := false
	err := api.BuildOptions{}.Apply(project)
	if err != nil {
		return false, err
	}
	for i, service := range project.Services {
		if service.Build == nil {
			continue
		}

		image := api.GetImageNameOrDefault(service, project.Name)
		_, localImagePresent := images[image]
		if localImagePresent && service.PullPolicy != types.PullPolicyBuild {
			service.Build = nil
			project.Services[i] = service
			continue
		}

		if service.Platform == "" {
			// let builder to build for default platform
			service.Build.Platforms = nil
		} else {
			service.Build.Platforms = []string{service.Platform}
		}
		project.Services[i] = service
		buildRequired = true
	}
	return buildRequired, nil
}

func (s *composeService) getLocalImagesDigests(ctx context.Context, project *types.Project) (map[string]string, error) {
	var imageNames []string
	for _, s := range project.Services {
		imgName := api.GetImageNameOrDefault(s, project.Name)
		if !utils.StringContains(imageNames, imgName) {
			imageNames = append(imageNames, imgName)
		}
	}
	imgs, err := s.getImages(ctx, imageNames)
	if err != nil {
		return nil, err
	}
	images := map[string]string{}
	for name, info := range imgs {
		images[name] = info.ID
	}

	for i, service := range project.Services {
		imgName := api.GetImageNameOrDefault(service, project.Name)
		digest, ok := images[imgName]
		if !ok {
			continue
		}
		if service.Platform != "" {
			platform, err := platforms.Parse(service.Platform)
			if err != nil {
				return nil, err
			}
			inspect, _, err := s.apiClient().ImageInspectWithRaw(ctx, digest)
			if err != nil {
				return nil, err
			}
			actual := specs.Platform{
				Architecture: inspect.Architecture,
				OS:           inspect.Os,
				Variant:      inspect.Variant,
			}
			if !platforms.NewMatcher(platform).Match(actual) {
				return nil, errors.Errorf("image with reference %s was found but does not match the specified platform: wanted %s, actual: %s",
					imgName, platforms.Format(platform), platforms.Format(actual))
			}
		}

		project.Services[i].CustomLabels.Add(api.ImageDigestLabel, digest)

	}

	return images, nil
}

func (s *composeService) toBuildOptions(project *types.Project, service types.ServiceConfig, options api.BuildOptions) (build.Options, error) {
	buildArgs := flatten(service.Build.Args.Resolve(envResolver(project.Environment)))

	for k, v := range storeutil.GetProxyConfig(s.dockerCli) {
		if _, ok := buildArgs[k]; !ok {
			buildArgs[k] = v
		}
	}

	plats, err := addPlatforms(project, service)
	if err != nil {
		return build.Options{}, err
	}

	cacheFrom, err := buildflags.ParseCacheEntry(service.Build.CacheFrom)
	if err != nil {
		return build.Options{}, err
	}
	cacheTo, err := buildflags.ParseCacheEntry(service.Build.CacheTo)
	if err != nil {
		return build.Options{}, err
	}

	sessionConfig := []session.Attachable{
		authprovider.NewDockerAuthProvider(s.configFile()),
	}
	if len(options.SSHs) > 0 || len(service.Build.SSH) > 0 {
		sshAgentProvider, err := sshAgentProvider(append(service.Build.SSH, options.SSHs...))
		if err != nil {
			return build.Options{}, err
		}
		sessionConfig = append(sessionConfig, sshAgentProvider)
	}

	if len(service.Build.Secrets) > 0 {
		secretsProvider, err := addSecretsConfig(project, service)
		if err != nil {
			return build.Options{}, err
		}
		sessionConfig = append(sessionConfig, secretsProvider)
	}

	tags := []string{api.GetImageNameOrDefault(service, project.Name)}
	if len(service.Build.Tags) > 0 {
		tags = append(tags, service.Build.Tags...)
	}
	var allow []entitlements.Entitlement
	if service.Build.Privileged {
		allow = append(allow, entitlements.EntitlementSecurityInsecure)
	}

	imageLabels := getImageBuildLabels(project, service)

	push := options.Push && service.Image != ""
	exports := []bclient.ExportEntry{{
		Type: "docker",
		Attrs: map[string]string{
			"load": "true",
			"push": fmt.Sprint(push),
		},
	}}
	if len(service.Build.Platforms) > 1 {
		exports = []bclient.ExportEntry{{
			Type: "image",
			Attrs: map[string]string{
				"push": fmt.Sprint(push),
			},
		}}
	}

	return build.Options{
		Inputs: build.Inputs{
			ContextPath:      service.Build.Context,
			DockerfileInline: service.Build.DockerfileInline,
			DockerfilePath:   dockerFilePath(service.Build.Context, service.Build.Dockerfile),
			NamedContexts:    toBuildContexts(service.Build.AdditionalContexts),
		},
		CacheFrom:   pb.CreateCaches(cacheFrom),
		CacheTo:     pb.CreateCaches(cacheTo),
		NoCache:     service.Build.NoCache,
		Pull:        service.Build.Pull,
		BuildArgs:   buildArgs,
		Tags:        tags,
		Target:      service.Build.Target,
		Exports:     exports,
		Platforms:   plats,
		Labels:      imageLabels,
		NetworkMode: service.Build.Network,
		ExtraHosts:  service.Build.ExtraHosts.AsList(),
		Session:     sessionConfig,
		Allow:       allow,
	}, nil
}

func flatten(in types.MappingWithEquals) types.Mapping {
	out := types.Mapping{}
	if len(in) == 0 {
		return out
	}
	for k, v := range in {
		if v == nil {
			continue
		}
		out[k] = *v
	}
	return out
}

func mergeArgs(m ...types.Mapping) types.Mapping {
	merged := types.Mapping{}
	for _, mapping := range m {
		for key, val := range mapping {
			merged[key] = val
		}
	}
	return merged
}

func dockerFilePath(ctxName string, dockerfile string) string {
	if dockerfile == "" {
		return ""
	}
	if urlutil.IsGitURL(ctxName) || filepath.IsAbs(dockerfile) {
		return dockerfile
	}
	return filepath.Join(ctxName, dockerfile)
}

func sshAgentProvider(sshKeys types.SSHConfig) (session.Attachable, error) {
	sshConfig := make([]sshprovider.AgentConfig, 0, len(sshKeys))
	for _, sshKey := range sshKeys {
		sshConfig = append(sshConfig, sshprovider.AgentConfig{
			ID:    sshKey.ID,
			Paths: []string{sshKey.Path},
		})
	}
	return sshprovider.NewSSHAgentProvider(sshConfig)
}

func addSecretsConfig(project *types.Project, service types.ServiceConfig) (session.Attachable, error) {
	var sources []secretsprovider.Source
	for _, secret := range service.Build.Secrets {
		config := project.Secrets[secret.Source]
		id := secret.Source
		if secret.Target != "" {
			id = secret.Target
		}
		switch {
		case config.File != "":
			sources = append(sources, secretsprovider.Source{
				ID:       id,
				FilePath: config.File,
			})
		case config.Environment != "":
			sources = append(sources, secretsprovider.Source{
				ID:  id,
				Env: config.Environment,
			})
		default:
			return nil, fmt.Errorf("build.secrets only supports environment or file-based secrets: %q", secret.Source)
		}
		if secret.UID != "" || secret.GID != "" || secret.Mode != nil {
			logrus.Warn("secrets `uid`, `gid` and `mode` are not supported by BuildKit, they will be ignored")
		}
	}
	store, err := secretsprovider.NewStore(sources)
	if err != nil {
		return nil, err
	}
	return secretsprovider.NewSecretProvider(store), nil
}

func addPlatforms(project *types.Project, service types.ServiceConfig) ([]specs.Platform, error) {
	plats, err := useDockerDefaultOrServicePlatform(project, service, false)
	if err != nil {
		return nil, err
	}

	for _, buildPlatform := range service.Build.Platforms {
		p, err := platforms.Parse(buildPlatform)
		if err != nil {
			return nil, err
		}
		if !utils.Contains(plats, p) {
			plats = append(plats, p)
		}
	}
	return plats, nil
}

func getImageBuildLabels(project *types.Project, service types.ServiceConfig) types.Labels {
	ret := make(types.Labels)
	if service.Build != nil {
		for k, v := range service.Build.Labels {
			ret.Add(k, v)
		}
	}

	ret.Add(api.VersionLabel, api.ComposeVersion)
	ret.Add(api.ProjectLabel, project.Name)
	ret.Add(api.ServiceLabel, service.Name)
	return ret
}

func toBuildContexts(additionalContexts types.Mapping) map[string]build.NamedContext {
	namedContexts := map[string]build.NamedContext{}
	for name, context := range additionalContexts {
		namedContexts[name] = build.NamedContext{Path: context}
	}
	return namedContexts
}

func useDockerDefaultPlatform(project *types.Project, platformList types.StringList) ([]specs.Platform, error) {
	var plats []specs.Platform
	if platform, ok := project.Environment["DOCKER_DEFAULT_PLATFORM"]; ok {
		if len(platformList) > 0 && !utils.StringContains(platformList, platform) {
			return nil, fmt.Errorf("the DOCKER_DEFAULT_PLATFORM %q value should be part of the service.build.platforms: %q", platform, platformList)
		}
		p, err := platforms.Parse(platform)
		if err != nil {
			return nil, err
		}
		plats = append(plats, p)
	}
	return plats, nil
}

func useDockerDefaultOrServicePlatform(project *types.Project, service types.ServiceConfig, useOnePlatform bool) ([]specs.Platform, error) {
	plats, err := useDockerDefaultPlatform(project, service.Build.Platforms)
	if (len(plats) > 0 && useOnePlatform) || err != nil {
		return plats, err
	}

	if service.Platform != "" {
		if len(service.Build.Platforms) > 0 && !utils.StringContains(service.Build.Platforms, service.Platform) {
			return nil, fmt.Errorf("service.platform %q should be part of the service.build.platforms: %q", service.Platform, service.Build.Platforms)
		}
		// User defined a service platform and no build platforms, so we should keep the one define on the service level
		p, err := platforms.Parse(service.Platform)
		if !utils.Contains(plats, p) {
			plats = append(plats, p)
		}
		return plats, err
	}
	return plats, nil
}
