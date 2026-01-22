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
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/remote"
	"github.com/docker/compose/v5/pkg/utils"
)

// LoadProject implements api.Compose.LoadProject
// It loads and validates a Compose project from configuration files.
func (s *composeService) LoadProject(ctx context.Context, options api.ProjectLoadOptions) (*types.Project, error) {
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

// collectIncludeEnvFiles scans top-level compose files for `include` entries
// and returns a list of resolved env_file paths declared on those includes.
// This implementation is intentionally small and defensive: it unmarshals the
// document to a generic map and extracts `include` -> `env_file` entries.
func collectIncludeEnvFiles(configPaths []string) ([]string, error) {
	var files []string
	for _, cp := range configPaths {
		content, err := os.ReadFile(cp)
		if err != nil {
			continue
		}
		var data map[string]interface{}
		if err := yaml.Unmarshal(content, &data); err != nil {
			continue
		}
		incRaw, ok := data["include"]
		if !ok {
			continue
		}
		incs, ok := incRaw.([]interface{})
		if !ok {
			continue
		}
		for _, inc := range incs {
			im, ok := inc.(map[string]interface{})
			if !ok {
				continue
			}
			envRaw, ok := im["env_file"]
			if !ok {
				continue
			}
			switch v := envRaw.(type) {
			case string:
				resolved := filepath.Join(filepath.Dir(cp), v)
				if _, err := os.Stat(resolved); err == nil {
					files = append(files, resolved)
				}
			case []interface{}:
				for _, e := range v {
					switch ee := e.(type) {
					case string:
						resolved := filepath.Join(filepath.Dir(cp), ee)
						if _, err := os.Stat(resolved); err == nil {
							files = append(files, resolved)
						}
					case map[string]interface{}:
						if p, ok := ee["path"].(string); ok {
							resolved := filepath.Join(filepath.Dir(cp), p)
							if _, err := os.Stat(resolved); err == nil {
								files = append(files, resolved)
							}
						}
					}
				}
			}
		}
	}
	return files, nil
}

// createRemoteLoaders creates Git and OCI remote loaders if not in offline mode
func (s *composeService) createRemoteLoaders(options api.ProjectLoadOptions) []loader.ResourceLoader {
	if options.Offline {
		return nil
	}
	git := remote.NewGitRemoteLoader(s.dockerCli, options.Offline)
	oci := remote.NewOCIRemoteLoader(s.dockerCli, options.Offline, options.OCI)
	return []loader.ResourceLoader{git, oci}
}

// buildProjectOptions constructs compose-go ProjectOptions from API options
func (s *composeService) buildProjectOptions(options api.ProjectLoadOptions, remoteLoaders []loader.ResourceLoader) (*cli.ProjectOptions, error) {
	opts := []cli.ProjectOptionsFn{
		cli.WithWorkingDirectory(options.WorkingDir),
		cli.WithOsEnv,
	}

	// Add PWD if not present
	if _, present := os.LookupEnv("PWD"); !present {
		if pwd, err := os.Getwd(); err == nil {
			opts = append(opts, cli.WithEnv([]string{"PWD=" + pwd}))
		}
	}

	// Add remote loaders
	for _, r := range remoteLoaders {
		opts = append(opts, cli.WithResourceLoader(r))
	}

	// Collect env_files declared on include: entries such as
	// include:
	//   - path: subproj/subcompose.yml
	//     env_file:
	//       - values.env
	// should apply values.env when evaluating the included compose file.
	includeEnvFiles, err := collectIncludeEnvFiles(options.ConfigPaths)
	if err != nil {
		return nil, err
	}

	// Merge CLI-provided env_files and include-provided env_files so they are
	// considered during project loading/interpolation.
	allEnvFiles := append([]string{}, options.EnvFiles...)
	allEnvFiles = append(allEnvFiles, includeEnvFiles...)

	opts = append(opts,
		// Load PWD/.env if present and no explicit --env-file has been set
		cli.WithEnvFiles(allEnvFiles...),
		// read dot env file to populate project environment
		cli.WithDotEnv,
		// get compose file path set by COMPOSE_FILE
		cli.WithConfigFileEnv,
		// if none was selected, get default compose.yaml file from current dir or parent folder
		cli.WithDefaultConfigPath,
		// .. and then, a project directory != PWD maybe has been set so let's load .env file
		cli.WithEnvFiles(allEnvFiles...), //nolint:gocritic // intentionally applying cli.WithEnvFiles twice.
		cli.WithDotEnv,                   //nolint:gocritic // intentionally applying cli.WithDotEnv twice.
		// eventually COMPOSE_PROFILES should have been set
		cli.WithDefaultProfiles(options.Profiles...),
		cli.WithName(options.ProjectName),
	)

	return cli.NewProjectOptions(options.ConfigPaths, append(options.ProjectOptionsFns, opts...)...)
}

// postProcessProject applies post-loading transformations to the project
func (s *composeService) postProcessProject(project *types.Project, options api.ProjectLoadOptions) (*types.Project, error) {
	if project.Name == "" {
		return nil, errors.New("project name can't be empty. Use ProjectName option to set a valid name")
	}

	project, err := project.WithServicesEnabled(options.Services...)
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
