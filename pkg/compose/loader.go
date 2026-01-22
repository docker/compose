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

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"gopkg.in/yaml.v3"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/remote"
	"github.com/docker/compose/v5/pkg/utils"
)

// LoadProject implements api.Compose.LoadProject
// It loads and validates a Compose project from configuration files.
func (s *composeService) LoadProject(ctx context.Context, options api.ProjectLoadOptions) (*types.Project, error) {
	// Extract env vars from include env_files to make them available for interpolation
	includeEnvVars, err := extractIncludeEnvVars(options.ConfigPaths, options.WorkingDir)
	if err != nil {
		return nil, err
	}

	// Temporarily set include env vars in OS environment for env_file interpolation
	envBackup := make(map[string]string)
	for k, v := range includeEnvVars {
		if oldVal, exists := os.LookupEnv(k); exists {
			envBackup[k] = oldVal
		}
		os.Setenv(k, v)
	}
	defer func() {
		// Restore original environment
		for k := range includeEnvVars {
			if oldVal, exists := envBackup[k]; exists {
				os.Setenv(k, oldVal)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

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

	opts = append(opts,
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

	return cli.NewProjectOptions(options.ConfigPaths, append(options.ProjectOptionsFns, opts...)...)
}

// extractIncludeEnvVars extracts environment variables from env_file directives in include directives
func extractIncludeEnvVars(configPaths []string, workingDir string) (map[string]string, error) {
	envFiles, err := extractIncludeEnvFilesFromFilePaths(configPaths, workingDir)
	if err != nil {
		return nil, err
	}
	vars := make(map[string]string)
	for _, envFile := range envFiles {
		fileVars, err := loadEnvFile(envFile)
		if err != nil {
			return nil, err
		}
		for k, v := range fileVars {
			vars[k] = v
		}
	}
	return vars, nil
}

// extractIncludeEnvFilesFromFilePaths extracts env_file paths from include directives in compose files
func extractIncludeEnvFilesFromFilePaths(configPaths []string, workingDir string) ([]string, error) {
	var envFiles []string
	for _, configPath := range configPaths {
		absPath := configPath
		if !filepath.IsAbs(configPath) {
			absPath = filepath.Join(workingDir, configPath)
		}
		files, err := extractIncludeEnvFilesFromFile(absPath)
		if err != nil {
			return nil, err
		}
		envFiles = append(envFiles, files...)
	}
	return envFiles, nil
}

// loadEnvFile loads environment variables from a .env file
func loadEnvFile(filePath string) (map[string]string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	vars := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			vars[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return vars, nil
}

// extractIncludeEnvFilesFromFile parses a compose file and extracts env_file paths from include directives
func extractIncludeEnvFilesFromFile(filePath string) ([]string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var compose map[string]interface{}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, err
	}
	includes, ok := compose["include"]
	if !ok {
		return nil, nil
	}
	includeList, ok := includes.([]interface{})
	if !ok {
		return nil, nil
	}
	var envFiles []string
	dir := filepath.Dir(filePath)
	for _, item := range includeList {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		envFileList, ok := itemMap["env_file"]
		if !ok {
			continue
		}
		files, ok := envFileList.([]interface{})
		if !ok {
			// Could be a single string
			if file, ok := envFileList.(string); ok {
				absFile := filepath.Join(dir, file)
				envFiles = append(envFiles, absFile)
			}
			continue
		}
		for _, f := range files {
			if file, ok := f.(string); ok {
				absFile := filepath.Join(dir, file)
				envFiles = append(envFiles, absFile)
			}
		}
	}
	return envFiles, nil
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
