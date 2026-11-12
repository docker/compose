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

// Package build implements the build of the images of a project's services,
// with BuildKit through the buildx bake plugin or with the classic builder.
package build

import (
	"context"
	"os/exec"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"

	"github.com/docker/compose/v5/internal/envresolver"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

// Backend is what the build needs from the engine it acts on.
type Backend interface {
	// CLI is the Docker CLI Compose runs in: the daemon client, the credentials,
	// the terminal output and the buildx plugin all come from it.
	CLI() command.Cli
	// Events receives the progress of the builds.
	Events() api.EventProcessor
	// DryRun tells whether the build only reports what it would do.
	DryRun() bool
	// ProxyConfig is the proxy settings handed to the builds as build arguments.
	ProxyConfig() map[string]*string
	// InDependencyOrder runs fn for every service of the project once the
	// services it depends on are done, with the concurrency the engine allows.
	InDependencyOrder(ctx context.Context, project *types.Project, fn func(ctx context.Context, service string) error) error
	// MaxConcurrency is the upper limit of concurrent operations, <= 0 for unlimited.
	MaxConcurrency() int
	// CanonicalBuiltDigest resolves the content digest to record for an image just built.
	CanonicalBuiltDigest(ctx context.Context, imageRef, platform, builderDigest string) string
	// Push pushes the images of the project.
	Push(ctx context.Context, project *types.Project, options api.PushOptions) error
	// PrepareShellOut sets the environment of a command Compose runs.
	PrepareShellOut(ctx context.Context, env types.Mapping, cmd *exec.Cmd) error
	// PropagateDockerEndpoint produces the environment that points a child CLI
	// plugin to the same Docker endpoint; its cleanup function must be called
	// once the child process is done.
	PropagateDockerEndpoint() ([]string, func(), error)
}

// Service builds images on a Backend.
type Service struct {
	backend Backend
}

// New creates a Service acting on the given Backend.
func New(backend Backend) *Service {
	return &Service{backend: backend}
}

// Build builds the services of the project that have something to build, and returns the digest of each image built by image name.
// Services whose image is already in localImages are skipped, unless their pull policy is "build".
func (s *Service) Build(ctx context.Context, project *types.Project, options api.BuildOptions, localImages map[string]api.ImageSummary) (map[string]string, error) {
	imageIDs := map[string]string{}
	serviceToBuild := types.Services{}

	var policy types.DependencyOption = types.IgnoreDependencies
	if options.Deps {
		policy = types.IncludeDependencies
	}

	if len(options.Services) == 0 {
		options.Services = project.ServiceNames()
	}

	// also include services used as additional_contexts with service: prefix
	options.Services = addBuildDependencies(options.Services, project)

	// Some build dependencies we just introduced may not be enabled
	var err error
	project, err = project.WithServicesEnabled(options.Services...)
	if err != nil {
		return nil, err
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
	if err != nil {
		return imageIDs, err
	}

	if len(serviceToBuild) == 0 {
		return imageIDs, nil
	}

	bake, err := buildWithBake(s.backend.CLI())
	if err != nil {
		return nil, err
	}
	if bake {
		return s.doBuildBake(ctx, project, serviceToBuild, options)
	}
	return s.doBuildClassic(ctx, project, serviceToBuild, options)
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
func resolveAndMergeBuildArgs(proxyConfig map[string]*string, project *types.Project, service types.ServiceConfig, options api.BuildOptions) types.MappingWithEquals {
	result := make(types.MappingWithEquals).
		OverrideBy(service.Build.Args).
		OverrideBy(options.Args).
		Resolve(envresolver.New(project.Environment))

	// proxy arguments do NOT override and should NOT have env resolution applied,
	// so they're handled last
	for k, v := range proxyConfig {
		if _, ok := result[k]; !ok {
			result[k] = v
		}
	}
	return result
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
