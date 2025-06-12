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
	"strings"
	"time"

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
	bclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	"github.com/moby/buildkit/util/entitlements"
	"github.com/moby/buildkit/util/progress/progressui"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	// required to get default driver registered
	_ "github.com/docker/buildx/driver/docker"
)

func (s *composeService) Build(ctx context.Context, project *types.Project, options api.BuildOptions) error {
	err := options.Apply(project)
	if err != nil {
		return err
	}
	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		return tracing.SpanWrapFunc("project/build", tracing.ProjectOptions(ctx, project),
			func(ctx context.Context) error {
				_, err := s.build(ctx, project, options, nil)
				return err
			})(ctx)
	}, s.stdinfo(), "Building")
}

//nolint:gocyclo
func (s *composeService) build(ctx context.Context, project *types.Project, options api.BuildOptions, localImages map[string]api.ImageSummary) (map[string]string, error) {
	imageIDs := map[string]string{}
	serviceToBuild := types.Services{}

	var policy types.DependencyOption = types.IgnoreDependencies
	if options.Deps {
		policy = types.IncludeDependencies
	}

	var err error
	if len(options.Services) > 0 {
		// As user requested some services to be built, also include those used as additional_contexts
		options.Services = addBuildDependencies(options.Services, project)
		// Some build dependencies we just introduced may not be enabled
		project, err = project.WithServicesEnabled(options.Services...)
		if err != nil {
			return nil, err
		}
	}
	project, err = project.WithSelectedServices(options.Services)
	if err != nil {
		return nil, err
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
		serviceToBuild[serviceName] = *service
		return nil
	}, policy)
	if err != nil || len(serviceToBuild) == 0 {
		return imageIDs, err
	}

	bake, err := buildWithBake(s.dockerCli)
	if err != nil {
		return nil, err
	}
	if bake || options.Print {
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("builder", "bake"))
		return s.doBuildBake(ctx, project, serviceToBuild, options)
	}

	// Not using bake, additional_context: service:xx is implemented by building images in dependency order
	project, err = project.WithServicesTransform(func(serviceName string, service types.ServiceConfig) (types.ServiceConfig, error) {
		if service.Build != nil {
			for _, c := range service.Build.AdditionalContexts {
				if t, found := strings.CutPrefix(c, types.ServicePrefix); found {
					if service.DependsOn == nil {
						service.DependsOn = map[string]types.ServiceDependency{}
					}
					service.DependsOn[t] = types.ServiceDependency{
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

	// Initialize buildkit nodes
	buildkitEnabled, err := s.dockerCli.BuildKitEnabled()
	if err != nil {
		return nil, err
	}
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
		if options.Progress == "" {
			options.Progress = os.Getenv("BUILDKIT_PROGRESS")
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

	cw := progress.ContextWriter(ctx)
	err = InDependencyOrder(ctx, project, func(ctx context.Context, name string) error {
		service, ok := serviceToBuild[name]
		if !ok {
			return nil
		}
		serviceName := fmt.Sprintf("Service %s", name)

		if !buildkitEnabled {
			trace.SpanFromContext(ctx).SetAttributes(attribute.String("builder", "classic"))
			cw.Event(progress.BuildingEvent(serviceName))
			id, err := s.doBuildClassic(ctx, project, service, options)
			if err != nil {
				return err
			}
			cw.Event(progress.BuiltEvent(serviceName))
			builtDigests[getServiceIndex(name)] = id

			if options.Push {
				return s.push(ctx, project, api.PushOptions{})
			}
			return nil
		}

		if options.Memory != 0 {
			_, _ = fmt.Fprintln(s.stderr(), "WARNING: --memory is not supported by BuildKit and will be ignored")
		}

		buildOptions, err := s.toBuildOptions(project, service, options)
		if err != nil {
			return err
		}

		trace.SpanFromContext(ctx).SetAttributes(attribute.String("builder", "buildkit"))
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
			service := project.Services[names[i]]
			imageRef := api.GetImageNameOrDefault(service, project.Name)
			imageIDs[imageRef] = imageDigest
			cw.Event(progress.BuiltEvent(names[i]))
		}
	}
	return imageIDs, err
}

func (s *composeService) ensureImagesExists(ctx context.Context, project *types.Project, buildOpts *api.BuildOptions, quietPull bool) error {
	for name, service := range project.Services {
		if service.Provider == nil && service.Image == "" && service.Build == nil {
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
					images[name] = api.ImageSummary{
						Repository:  name,
						ID:          digest,
						LastTagTime: time.Now(),
					}
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
		img, ok := images[image]
		if ok {
			service.CustomLabels.Add(api.ImageDigestLabel, img.ID)
		}
		project.Services[name] = service
	}
	return nil
}

func (s *composeService) getLocalImagesDigests(ctx context.Context, project *types.Project) (map[string]api.ImageSummary, error) {
	imageNames := utils.Set[string]{}
	for _, s := range project.Services {
		imageNames.Add(api.GetImageNameOrDefault(s, project.Name))
		for _, volume := range s.Volumes {
			if volume.Type == types.VolumeTypeImage {
				imageNames.Add(volume.Source)
			}
		}
	}
	imgs, err := s.getImageSummaries(ctx, imageNames.Elements())
	if err != nil {
		return nil, err
	}

	for i, service := range project.Services {
		imgName := api.GetImageNameOrDefault(service, project.Name)
		img, ok := imgs[imgName]
		if !ok {
			continue
		}
		if service.Platform != "" {
			platform, err := platforms.Parse(service.Platform)
			if err != nil {
				return nil, err
			}
			inspect, err := s.apiClient().ImageInspect(ctx, img.ID)
			if err != nil {
				return nil, err
			}
			actual := specs.Platform{
				Architecture: inspect.Architecture,
				OS:           inspect.Os,
				Variant:      inspect.Variant,
			}
			if !platforms.NewMatcher(platform).Match(actual) {
				logrus.Debugf("local image %s doesn't match expected platform %s", service.Image, service.Platform)
				// there is a local image, but it's for the wrong platform, so
				// pretend it doesn't exist so that we can pull/build an image
				// for the correct platform instead
				delete(imgs, imgName)
			}
		}

		project.Services[i].CustomLabels.Add(api.ImageDigestLabel, img.ID)

	}

	return imgs, nil
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
func resolveAndMergeBuildArgs(dockerCli command.Cli, project *types.Project, service types.ServiceConfig, opts api.BuildOptions) types.MappingWithEquals {
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
		authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{
			ConfigFile: s.configFile(),
		}),
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
		allow = append(allow, entitlements.EntitlementSecurityInsecure.String())
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

	attests := map[string]*string{}
	if !options.Provenance {
		attests["provenance"] = nil
	}

	return build.Options{
		Inputs: build.Inputs{
			ContextPath:      service.Build.Context,
			DockerfileInline: service.Build.DockerfileInline,
			DockerfilePath:   dockerFilePath(service.Build.Context, service.Build.Dockerfile),
			NamedContexts:    toBuildContexts(service, project),
		},
		CacheFrom:    pb.CreateCaches(cacheFrom.ToPB()),
		CacheTo:      pb.CreateCaches(cacheTo.ToPB()),
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
		Attests:      attests,
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

func toBuildContexts(service types.ServiceConfig, project *types.Project) map[string]build.NamedContext {
	namedContexts := map[string]build.NamedContext{}
	for name, contextPath := range service.Build.AdditionalContexts {
		if strings.HasPrefix(contextPath, types.ServicePrefix) {
			// image we depend on has been built previously, as we run in dependency order.
			// so we convert the service reference into an image reference
			target := contextPath[len(types.ServicePrefix):]
			image := api.GetImageNameOrDefault(project.Services[target], project.Name)
			contextPath = "docker-image://" + image
		}
		namedContexts[name] = build.NamedContext{Path: contextPath}
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

func addBuildDependencies(services []string, project *types.Project) []string {
	servicesWithDependencies := utils.NewSet(services...)
	for _, service := range services {
		s, ok := project.Services[service]
		if !ok {
			s = project.DisabledServices[service]
		}
		b := s.Build
		if b != nil {
			for _, target := range b.AdditionalContexts {
				if s, found := strings.CutPrefix(target, types.ServicePrefix); found {
					servicesWithDependencies.Add(s)
				}
			}
		}
	}
	if len(servicesWithDependencies) > len(services) {
		return addBuildDependencies(servicesWithDependencies.Elements(), project)
	}
	return servicesWithDependencies.Elements()
}
