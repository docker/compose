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

package projectloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/remote"
	"github.com/docker/compose/v5/pkg/utils"
)

// LoadProject implements api.Compose.LoadProject
// It loads and validates a Compose project from configuration files.
func (s *Service) LoadProject(ctx context.Context, options api.ProjectLoadOptions) (*types.Project, error) {
	// Setup remote loaders (Git, OCI)
	remoteLoaders := s.createRemoteLoaders(options)

	projectOptions, err := s.buildProjectOptions(options, remoteLoaders)
	if err != nil {
		return nil, err
	}

	// Register all user-provided listeners (e.g., for metrics collection)
	for _, listener := range options.LoadListeners {
		if listener != nil {
			projectOptions.WithListeners(listener)
		}
	}

	if options.Compatibility || utils.StringToBool(projectOptions.Environment[api.ComposeCompatibility]) {
		api.Separator = "_"
	}

	if len(options.ExcludedProfiles) == 0 {
		options.ExcludedProfiles = splitProfiles(projectOptions.Environment[api.ComposeExcludedProfiles])
	}

	project, err := projectOptions.LoadProject(ctx)
	if err != nil {
		return nil, err
	}

	// Post-processing: service selection, environment resolution, etc.
	project, err = s.postProcessProject(project, options)
	if err != nil {
		return nil, err
	}

	return project, nil
}

// createRemoteLoaders creates Git and OCI remote loaders if not in offline mode
func (s *Service) createRemoteLoaders(options api.ProjectLoadOptions) []loader.ResourceLoader {
	if options.Offline {
		return nil
	}
	git := remote.NewGitRemoteLoader(s.backend.CLI(), options.Offline)
	oci := remote.NewOCIRemoteLoader(s.backend.CLI(), options.Offline, options.OCI)
	return []loader.ResourceLoader{git, oci}
}

// buildProjectOptions constructs compose-go ProjectOptions from API options
func (s *Service) buildProjectOptions(options api.ProjectLoadOptions, remoteLoaders []loader.ResourceLoader) (*cli.ProjectOptions, error) {
	projectOptionsFns := []cli.ProjectOptionsFn{
		cli.WithWorkingDirectory(options.WorkingDir),
		cli.WithOsEnv,
	}

	// Add PWD if not present
	if _, present := os.LookupEnv("PWD"); !present {
		if pwd, err := os.Getwd(); err == nil {
			projectOptionsFns = append(projectOptionsFns, cli.WithEnv([]string{"PWD=" + pwd}))
		}
	}

	// Add remote loaders
	for _, r := range remoteLoaders {
		projectOptionsFns = append(projectOptionsFns, cli.WithResourceLoader(r))
	}

	projectOptionsFns = append(projectOptionsFns,
		// Load PWD/.env if present and no explicit --env-file has been set
		cli.WithEnvFiles(options.EnvFiles...),
		// read dot env file to populate project environment
		cli.WithDotEnv,
		// get compose file path set by COMPOSE_FILE
		cli.WithConfigFileEnv,
		// if none was selected, get default compose.yaml file from current dir or parent folder
		cli.WithDefaultConfigPath,
		// .. and then, a project directory != PWD maybe has been set so let's load .env file
		cli.WithEnvFiles(options.EnvFiles...), //nolint:gocritic // intentionally applying cli.WithEnvFiles twice.
		cli.WithDotEnv,                        //nolint:gocritic // intentionally applying cli.WithDotEnv twice.
		// eventually COMPOSE_PROFILES should have been set
		cli.WithDefaultProfiles(options.Profiles...),
		cli.WithName(options.ProjectName),
	)

	if options.OnUnsupportedAttribute != nil {
		projectOptionsFns = append(projectOptionsFns, unsupportedAttributesLoadOption(options.OnUnsupportedAttribute))
	}

	return cli.NewProjectOptions(options.ConfigPaths, append(options.ProjectOptionsFns, projectOptionsFns...)...)
}

// splitProfiles parses a comma-separated list of profiles, as
// COMPOSE_PROFILES does, ignoring blank entries.
func splitProfiles(value string) []string {
	var profiles []string
	for p := range strings.SplitSeq(value, ",") {
		if p = strings.TrimSpace(p); p != "" {
			profiles = append(profiles, p)
		}
	}
	return profiles
}

// excludeProfiles disables the enabled services and jobs attached to any of
// the excluded profiles. It runs once profiles have been activated, so it
// only ever removes from that selection. Excluded services move to
// DisabledServices — as an inactive profile does — so they are not mistaken
// for orphans, and a required dependency on one of them is reported rather
// than silently dropped. This is why project.WithServicesDisabled, which also
// drops the depends_on references to the disabled services, is not used here.
func excludeProfiles(project *types.Project, excluded []string, selected []string) (*types.Project, error) {
	if len(excluded) == 0 {
		return project, nil
	}
	isExcluded := func(profiles []string) (string, bool) {
		for _, p := range profiles {
			if slices.Contains(excluded, p) {
				return p, true
			}
		}
		return "", false
	}

	for _, name := range selected {
		if s, ok := project.Services[name]; ok {
			if p, ok := isExcluded(s.Profiles); ok {
				return nil, fmt.Errorf("service %q belongs to excluded profile %q", name, p)
			}
		}
	}

	excludedServices := map[string]string{}
	for name, s := range project.Services {
		if p, ok := isExcluded(s.Profiles); ok {
			if project.DisabledServices == nil {
				project.DisabledServices = types.Services{}
			}
			project.DisabledServices[name] = s
			delete(project.Services, name)
			excludedServices[name] = p
		}
	}
	if err := checkExcludedDependencies(project, excludedServices, selected); err != nil {
		return nil, err
	}

	for name, j := range project.Jobs {
		if _, ok := isExcluded(j.Profiles); ok {
			if project.DisabledJobs == nil {
				project.DisabledJobs = types.Jobs{}
			}
			project.DisabledJobs[name] = j
			delete(project.Jobs, name)
		}
	}
	// an optional dependency on an excluded service is dropped, as it is for
	// a service disabled by an inactive profile; required ones are kept so
	// they are reported
	return project.WithoutUnresolvedOptionalDependencies(), nil
}

// checkExcludedDependencies reports a required dependency on a service of an
// excluded profile, from the selected services (all of them if none is
// selected) or from any service they transitively depend on. Services left
// out of the selection are not checked, as they are dropped later on.
func checkExcludedDependencies(project *types.Project, excluded map[string]string, selected []string) error {
	if len(excluded) == 0 {
		return nil
	}
	pending := slices.Clone(selected)
	if len(pending) == 0 {
		pending = project.ServiceNames()
	}
	visited := map[string]bool{}
	for len(pending) > 0 {
		name := pending[0]
		pending = pending[1:]
		if visited[name] {
			continue
		}
		visited[name] = true
		for dep, cfg := range project.Services[name].DependsOn {
			if p, ok := excluded[dep]; ok && cfg.Required {
				return fmt.Errorf("service %q depends on service %q, which belongs to excluded profile %q", name, dep, p)
			}
			pending = append(pending, dep)
		}
	}
	return nil
}

// postProcessProject applies post-loading transformations to the project
func (s *Service) postProcessProject(project *types.Project, options api.ProjectLoadOptions) (*types.Project, error) {
	if project.Name == "" {
		return nil, errors.New("project name can't be empty. Use ProjectName option to set a valid name")
	}

	project, err := project.WithServicesEnabled(options.Services...)
	if err != nil {
		return nil, err
	}

	project, err = excludeProfiles(project, options.ExcludedProfiles, options.Services)
	if err != nil {
		return nil, err
	}

	// Add custom labels
	for name, s := range project.Services {
		s.CustomLabels = map[string]string{
			api.ProjectLabel:     project.Name,
			api.ServiceLabel:     name,
			api.VersionLabel:     api.ComposeVersion,
			api.WorkingDirLabel:  project.WorkingDir,
			api.ConfigFilesLabel: strings.Join(project.ComposeFiles, ","),
			api.OneoffLabel:      "False",
		}
		if len(options.EnvFiles) != 0 {
			s.CustomLabels[api.EnvironmentFileLabel] = strings.Join(options.EnvFiles, ",")
		}
		project.Services[name] = s
	}

	project, err = project.WithSelectedServices(options.Services)
	if err != nil {
		return nil, err
	}

	// Remove unnecessary resources if not All
	if !options.All {
		project = project.WithoutUnnecessaryResources()
	}

	return project, nil
}
