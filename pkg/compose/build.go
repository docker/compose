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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/platforms"
	"github.com/docker/buildx/build"
	"github.com/docker/buildx/builder"
	"github.com/docker/buildx/controller/pb"
	"github.com/docker/buildx/store/storeutil"
	"github.com/docker/buildx/util/buildflags"
	xprogress "github.com/docker/buildx/util/progress"
	"github.com/docker/cli/cli/command"
	cliopts "github.com/docker/cli/opts"
	"github.com/docker/compose/v2/internal/tracing"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/utils"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/builder/remotecontext/urlutil"
	bclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	"github.com/moby/buildkit/util/entitlements"
	"github.com/moby/buildkit/util/progress/progressui"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"

	// required to get default driver registered
	_ "github.com/docker/buildx/driver/docker"
)

func (s *composeService) Build(ctx context.Context, project *types.Project, options api.BuildOptions) error {
	err := options.Apply(project)
	if err != nil {
		return err
	}
	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		_, err := s.build(ctx, project, options, nil)
		return err
	}, s.stdinfo(), "Building")
}

type serviceToBuild struct {
	name    string
	service types.ServiceConfig
}

//nolint:gocyclo
func (s *composeService) build(ctx context.Context, project *types.Project, options api.BuildOptions, localImages map[string]string) (map[string]string, error) {
	buildkitEnabled, err := s.dockerCli.BuildKitEnabled()
	if err != nil {
		return nil, err
	}

	imageIDs := map[string]string{}
	serviceToBeBuild := map[string]serviceToBuild{}

	var policy types.DependencyOption = types.IgnoreDependencies
	if options.Deps {
		policy = types.IncludeDependencies
	}
	err = project.ForEachService(options.Services, func(serviceName string, service *types.ServiceConfig) error {
		if service.Build == nil {
			return nil
		}
		image := api.GetImageNameOrDefault(*service, project.Name)
		_, localImagePresent := localImages[image]
		if localImagePresent && service.PullPolicy != types.PullPolicyBuild {
			return nil
		}
		serviceToBeBuild[serviceName] = serviceToBuild{name: serviceName, service: *service}
		return nil
	}, policy)
	if err != nil || len(serviceToBeBuild) == 0 {
		return imageIDs, err
	}

	// Initialize buildkit nodes
	var (
		b     *builder.Builder
		nodes []builder.Node
		w     *xprogress.Printer
	)
	if buildkitEnabled {
		builderName := options.Builder
		if builderName == "" {
			builderName = os.Getenv("BUILDX_BUILDER")
		}
		b, err = builder.New(s.dockerCli, builder.WithName(builderName))
		if err != nil {
			return nil, err
		}

		nodes, err = b.LoadNodes(ctx)
		if err != nil {
			return nil, err
		}

		// Progress needs its own context that lives longer than the
		// build one otherwise it won't read all the messages from
		// build and will lock
		progressCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if options.Quiet {
			options.Progress = progress.ModeQuiet
		}
		w, err = xprogress.NewPrinter(progressCtx, os.Stdout, progressui.DisplayMode(options.Progress),
			xprogress.WithDesc(
				fmt.Sprintf("building with %q instance using %s driver", b.Name, b.Driver),
				fmt.Sprintf("%s:%s", b.Driver, b.Name),
			))

		if err != nil {
			return nil, err
		}
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
		serviceToBuild, ok := serviceToBeBuild[name]
		if !ok {
			return nil
		}
		service := serviceToBuild.service

		if !buildkitEnabled {
			id, err := s.doBuildClassic(ctx, project, service, options)
			if err != nil {
				return err
			}
			builtDigests[getServiceIndex(name)] = id

			if options.Push {
				return s.push(ctx, project, api.PushOptions{})
			}
			return nil
		}

		if options.Memory != 0 {
			fmt.Fprintln(s.stderr(), "WARNING: --memory is not supported by BuildKit and will be ignored")
		}

		buildOptions, err := s.toBuildOptions(project, service, options)
		if err != nil {
			return err
		}

		digest, err := s.doBuildBuildkit(ctx, name, buildOptions, w, nodes)
		if err != nil {
			return err
		}
		builtDigests[getServiceIndex(name)] = digest

		return nil
	}, func(traversal *graphTraversal) {
		traversal.maxConcurrency = s.maxConcurrency
	})

	// enforce all build event get consumed
	if buildkitEnabled {
		if errw := w.Wait(); errw != nil {
			return nil, errw
		}
	}

	if err != nil {
		return nil, err
	}

	for i, imageDigest := range builtDigests {
		if imageDigest != "" {
			imageRef := api.GetImageNameOrDefault(project.Services[names[i]], project.Name)
			imageIDs[imageRef] = imageDigest
		}
	}
	return imageIDs, err
}

func (s *composeService) ensureImagesExists(ctx context.Context, project *types.Project, buildOpts *api.BuildOptions, quietPull bool) error {
	for name, service := range project.Services {
		if service.Image == "" && service.Build == nil {
			return fmt.Errorf("invalid service %q. Must specify either image or build", name)
		}
	}

	images, err := s.getLocalImagesDigests(ctx, project)
	if err != nil {
		return err
	}

	err = tracing.SpanWrapFunc("project/pull", tracing.ProjectOptions(ctx, project),
		func(ctx context.Context) error {
			return s.pullRequiredImages(ctx, project, images, quietPull)
		},
	)(ctx)
	if err != nil {
		return err
	}

	if buildOpts != nil {
		err = tracing.SpanWrapFunc("project/build", tracing.ProjectOptions(ctx, project),
			func(ctx context.Context) error {
				builtImages, err := s.build(ctx, project, *buildOpts, images)
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
	for name, service := range project.Services {
		image := api.GetImageNameOrDefault(service, project.Name)
		digest, ok := images[image]
		if ok {
			if service.Labels == nil {
				service.Labels = types.Labels{}
			}
			service.CustomLabels.Add(api.ImageDigestLabel, digest)
		}
		project.Services[name] = service
	}
	return nil
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
				// there is a local image, but it's for the wrong platform, so
				// pretend it doesn't exist so that we can pull/build an image
				// for the correct platform instead
				delete(images, imgName)
			}
		}

		project.Services[i].CustomLabels.Add(api.ImageDigestLabel, digest)

	}

	return images, nil
}

// resolveAndMergeBuildArgs returns the final set of build arguments to use for the service image build.
//
// First, args directly defined via `build.args` in YAML are considered.
// Then, any explicitly passed args in opts (e.g. via `--build-arg` on the CLI) are merged, overwriting any
// keys that already exist.
// Next, any keys without a value are resolved using the project environment.
//
// Finally, standard proxy variables based on the Docker client configuration are added, but will not overwrite
// any values if already present.
func resolveAndMergeBuildArgs(
	dockerCli command.Cli,
	project *types.Project,
	service types.ServiceConfig,
	opts api.BuildOptions,
) types.MappingWithEquals {
	result := make(types.MappingWithEquals).
		OverrideBy(service.Build.Args).
		OverrideBy(opts.Args).
		Resolve(envResolver(project.Environment))

	// proxy arguments do NOT override and should NOT have env resolution applied,
	// so they're handled last
	for k, v := range storeutil.GetProxyConfig(dockerCli) {
		if _, ok := result[k]; !ok {
			v := v
			result[k] = &v
		}
	}
	return result
}

func (s *composeService) toBuildOptions(project *types.Project, service types.ServiceConfig, options api.BuildOptions) (build.Options, error) {
	plats, err := parsePlatforms(service)
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
		authprovider.NewDockerAuthProvider(s.configFile(), nil),
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

	allow, err := buildflags.ParseEntitlements(service.Build.Entitlements)
	if err != nil {
		return build.Options{}, err
	}
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

	sp, err := build.ReadSourcePolicy()
	if err != nil {
		return build.Options{}, err
	}

	return build.Options{
		Inputs: build.Inputs{
			ContextPath:      service.Build.Context,
			DockerfileInline: service.Build.DockerfileInline,
			DockerfilePath:   dockerFilePath(service.Build.Context, service.Build.Dockerfile),
			NamedContexts:    toBuildContexts(service.Build.AdditionalContexts),
		},
		CacheFrom:    pb.CreateCaches(cacheFrom),
		CacheTo:      pb.CreateCaches(cacheTo),
		NoCache:      service.Build.NoCache,
		Pull:         service.Build.Pull,
		BuildArgs:    flatten(resolveAndMergeBuildArgs(s.dockerCli, project, service, options)),
		Tags:         tags,
		Target:       service.Build.Target,
		Exports:      exports,
		Platforms:    plats,
		Labels:       imageLabels,
		NetworkMode:  service.Build.Network,
		ExtraHosts:   service.Build.ExtraHosts.AsList(":"),
		Ulimits:      toUlimitOpt(service.Build.Ulimits),
		Session:      sessionConfig,
		Allow:        allow,
		SourcePolicy: sp,
	}, nil
}

func toUlimitOpt(ulimits map[string]*types.UlimitsConfig) *cliopts.UlimitOpt {
	ref := map[string]*container.Ulimit{}
	for _, limit := range toUlimits(ulimits) {
		ref[limit.Name] = &container.Ulimit{
			Name: limit.Name,
			Hard: limit.Hard,
			Soft: limit.Soft,
		}
	}
	return cliopts.NewUlimitOpt(&ref)
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

func parsePlatforms(service types.ServiceConfig) ([]specs.Platform, error) {
	if service.Build == nil || len(service.Build.Platforms) == 0 {
		return nil, nil
	}

	var errs []error
	ret := make([]specs.Platform, len(service.Build.Platforms))
	for i := range service.Build.Platforms {
		p, err := platforms.Parse(service.Build.Platforms[i])
		if err != nil {
			errs = append(errs, err)
		} else {
			ret[i] = p
		}
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return ret, nil
}
