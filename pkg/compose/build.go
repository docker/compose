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
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/containerd/platforms"
	"github.com/docker/buildx/build"
	_ "github.com/docker/buildx/driver/docker" // required to get default driver registered
	"github.com/docker/buildx/util/buildflags"
	xprogress "github.com/docker/buildx/util/progress"
	"github.com/docker/docker/pkg/urlutil"
	bclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	specs "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/utils"
)

func (s *composeService) Build(ctx context.Context, project *types.Project, options api.BuildOptions) error {
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.build(ctx, project, options)
	})
}

func (s *composeService) build(ctx context.Context, project *types.Project, options api.BuildOptions) error {
	opts := map[string]build.Options{}
	var imagesToBuild []string

	args := flatten(options.Args.Resolve(envResolver(project.Environment)))

	services, err := project.GetServices(options.Services...)
	if err != nil {
		return err
	}

	for _, service := range services {
		if service.Build == nil {
			continue
		}
		imageName := api.GetImageNameOrDefault(service, project.Name)
		imagesToBuild = append(imagesToBuild, imageName)
		buildOptions, err := s.toBuildOptions(project, service, imageName, options.SSHs)
		if err != nil {
			return err
		}
		buildOptions.Pull = options.Pull
		buildOptions.BuildArgs = mergeArgs(buildOptions.BuildArgs, args)
		buildOptions.NoCache = options.NoCache
		buildOptions.CacheFrom, err = buildflags.ParseCacheEntry(service.Build.CacheFrom)
		if err != nil {
			return err
		}

		for _, image := range service.Build.CacheFrom {
			buildOptions.CacheFrom = append(buildOptions.CacheFrom, bclient.CacheOptionsEntry{
				Type:  "registry",
				Attrs: map[string]string{"ref": image},
			})
		}
		opts[imageName] = buildOptions
	}

	_, err = s.doBuild(ctx, project, opts, options.Progress)
	if err == nil {
		if len(imagesToBuild) > 0 && !options.Quiet {
			utils.DisplayScanSuggestMsg()
		}
	}

	return err
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

	err = s.pullRequiredImages(ctx, project, images, quietPull)
	if err != nil {
		return err
	}

	mode := xprogress.PrinterModeAuto
	if quietPull {
		mode = xprogress.PrinterModeQuiet
	}
	opts, err := s.getBuildOptions(project, images)
	if err != nil {
		return err
	}
	builtImages, err := s.doBuild(ctx, project, opts, mode)
	if err != nil {
		return err
	}

	if len(builtImages) > 0 {
		utils.DisplayScanSuggestMsg()
	}
	for name, digest := range builtImages {
		images[name] = digest
	}
	// set digest as com.docker.compose.image label so we can detect outdated containers
	for i, service := range project.Services {
		image := api.GetImageNameOrDefault(service, project.Name)
		digest, ok := images[image]
		if ok {
			if project.Services[i].Labels == nil {
				project.Services[i].Labels = types.Labels{}
			}
			project.Services[i].CustomLabels[api.ImageDigestLabel] = digest
		}
	}
	return nil
}

func (s *composeService) getBuildOptions(project *types.Project, images map[string]string) (map[string]build.Options, error) {
	opts := map[string]build.Options{}
	for _, service := range project.Services {
		if service.Image == "" && service.Build == nil {
			return nil, fmt.Errorf("invalid service %q. Must specify either image or build", service.Name)
		}
		imageName := api.GetImageNameOrDefault(service, project.Name)
		_, localImagePresent := images[imageName]

		if service.Build != nil {
			if localImagePresent && service.PullPolicy != types.PullPolicyBuild {
				continue
			}
			opt, err := s.toBuildOptions(project, service, imageName, []types.SSHKey{})
			if err != nil {
				return nil, err
			}
			opts[imageName] = opt
			continue
		}
	}
	return opts, nil

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

	for i := range project.Services {
		imgName := api.GetImageNameOrDefault(project.Services[i], project.Name)
		digest, ok := images[imgName]
		if ok {
			project.Services[i].CustomLabels.Add(api.ImageDigestLabel, digest)
		}
	}

	return images, nil
}

func (s *composeService) doBuild(ctx context.Context, project *types.Project, opts map[string]build.Options, mode string) (map[string]string, error) {
	if len(opts) == 0 {
		return nil, nil
	}
	if buildkitEnabled, err := s.dockerCli.BuildKitEnabled(); err != nil || !buildkitEnabled {
		return s.doBuildClassic(ctx, project, opts)
	}
	return s.doBuildBuildkit(ctx, project, opts, mode)
}

func (s *composeService) toBuildOptions(project *types.Project, service types.ServiceConfig, imageTag string, sshKeys []types.SSHKey) (build.Options, error) {
	var tags []string
	tags = append(tags, imageTag)

	buildArgs := flatten(service.Build.Args.Resolve(envResolver(project.Environment)))

	var plats []specs.Platform
	if platform, ok := project.Environment["DOCKER_DEFAULT_PLATFORM"]; ok {
		p, err := platforms.Parse(platform)
		if err != nil {
			return build.Options{}, err
		}
		plats = append(plats, p)
	}
	if service.Platform != "" {
		p, err := platforms.Parse(service.Platform)
		if err != nil {
			return build.Options{}, err
		}
		plats = append(plats, p)
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
		authprovider.NewDockerAuthProvider(s.stderr()),
	}
	if len(sshKeys) > 0 || len(service.Build.SSH) > 0 {
		sshAgentProvider, err := sshAgentProvider(append(service.Build.SSH, sshKeys...))
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

	if len(service.Build.Tags) > 0 {
		tags = append(tags, service.Build.Tags...)
	}

	return build.Options{
		Inputs: build.Inputs{
			ContextPath:    service.Build.Context,
			DockerfilePath: dockerFilePath(service.Build.Context, service.Build.Dockerfile),
		},
		CacheFrom:   cacheFrom,
		CacheTo:     cacheTo,
		NoCache:     service.Build.NoCache,
		Pull:        service.Build.Pull,
		BuildArgs:   buildArgs,
		Tags:        tags,
		Target:      service.Build.Target,
		Exports:     []bclient.ExportEntry{{Type: "image", Attrs: map[string]string{}}},
		Platforms:   plats,
		Labels:      service.Build.Labels,
		NetworkMode: service.Build.Network,
		ExtraHosts:  service.Build.ExtraHosts.AsList(),
		Session:     sessionConfig,
	}, nil
}

func flatten(in types.MappingWithEquals) types.Mapping {
	if len(in) == 0 {
		return nil
	}
	out := types.Mapping{}
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
		switch {
		case config.File != "":
			sources = append(sources, secretsprovider.Source{
				ID:       secret.Source,
				FilePath: config.File,
			})
		case config.Environment != "":
			sources = append(sources, secretsprovider.Source{
				ID:  secret.Source,
				Env: config.Environment,
			})
		default:
			return nil, fmt.Errorf("build.secrets only supports environment or file-based secrets: %q", secret.Source)
		}
	}
	store, err := secretsprovider.NewStore(sources)
	if err != nil {
		return nil, err
	}
	return secretsprovider.NewSecretProvider(store), nil
}
