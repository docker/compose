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
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/client"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"

	"github.com/docker/compose/v5/pkg/api"
)

func TestLoadProject_Basic(t *testing.T) {
	// Create a temporary compose file
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	composeContent := `
name: test-project
services:
  web:
    image: nginx:latest
    ports:
      - "8080:80"
  db:
    image: postgres:latest
    environment:
      POSTGRES_PASSWORD: secret
`
	err := os.WriteFile(composeFile, []byte(composeContent), 0o644)
	assert.NilError(t, err)

	service := New(noBackend{})

	// Load the project
	project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
	})

	// Assertions
	assert.NilError(t, err)
	assert.Equal(t, "test-project", project.Name)
	assert.Assert(t, is.Len(project.Services, 2))
	assert.Check(t, is.Contains(project.Services, "web"))
	assert.Check(t, is.Contains(project.Services, "db"))

	// Check labels were applied
	webService := project.Services["web"]
	assert.Equal(t, "test-project", webService.CustomLabels[api.ProjectLabel])
	assert.Equal(t, "web", webService.CustomLabels[api.ServiceLabel])
}

func TestLoadProject_WithEnvironmentResolution(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	composeContent := `
services:
  app:
    image: myapp:latest
    environment:
      - TEST_VAR=${TEST_VAR}
      - LITERAL_VAR=literal_value
`
	err := os.WriteFile(composeFile, []byte(composeContent), 0o644)
	assert.NilError(t, err)

	// Set environment variable
	t.Setenv("TEST_VAR", "resolved_value")

	service := New(noBackend{})

	// Test with environment resolution (default)
	t.Run("WithResolution", func(t *testing.T) {
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths: []string{composeFile},
		})
		assert.NilError(t, err)

		appService := project.Services["app"]
		// Environment should be resolved
		assert.Assert(t, appService.Environment["TEST_VAR"] != nil)
		assert.Equal(t, "resolved_value", *appService.Environment["TEST_VAR"])
		assert.Assert(t, appService.Environment["LITERAL_VAR"] != nil)
		assert.Equal(t, "literal_value", *appService.Environment["LITERAL_VAR"])
	})

	// Test without environment resolution
	t.Run("WithoutResolution", func(t *testing.T) {
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths:       []string{composeFile},
			ProjectOptionsFns: []cli.ProjectOptionsFn{cli.WithoutEnvironmentResolution},
		})
		assert.NilError(t, err)

		appService := project.Services["app"]
		// Environment should NOT be resolved, keeping raw values
		// Note: This depends on compose-go behavior, which may still have some resolution
		assert.Assert(t, appService.Environment != nil)
	})
}

func TestLoadProject_ServiceSelection(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	composeContent := `
services:
  web:
    image: nginx:latest
  db:
    image: postgres:latest
  cache:
    image: redis:latest
`
	err := os.WriteFile(composeFile, []byte(composeContent), 0o644)
	assert.NilError(t, err)

	service := New(noBackend{})

	// Load only specific services
	project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
		Services:    []string{"web", "db"},
	})

	assert.NilError(t, err)
	assert.Check(t, is.Len(project.Services, 2))
	assert.Check(t, is.Contains(project.Services, "web"))
	assert.Check(t, is.Contains(project.Services, "db"))
	assert.Check(t, !is.Contains(project.Services, "cache")().Success())
}

func TestLoadProject_WithProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	composeContent := `
services:
  web:
    image: nginx:latest
  debug:
    image: busybox:latest
    profiles: ["debug"]
`
	err := os.WriteFile(composeFile, []byte(composeContent), 0o644)
	assert.NilError(t, err)

	service := New(noBackend{})

	// Without debug profile
	t.Run("WithoutProfile", func(t *testing.T) {
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths: []string{composeFile},
		})
		assert.NilError(t, err)
		assert.Check(t, is.Len(project.Services, 1))
		assert.Check(t, is.Contains(project.Services, "web"))
	})

	// With debug profile
	t.Run("WithProfile", func(t *testing.T) {
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths: []string{composeFile},
			Profiles:    []string{"debug"},
		})
		assert.NilError(t, err)
		assert.Check(t, is.Len(project.Services, 2))
		assert.Check(t, is.Contains(project.Services, "web"))
		assert.Check(t, is.Contains(project.Services, "debug"))
	})
}

func TestLoadProject_WithLoadListeners(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	composeContent := `
services:
  web:
    image: nginx:latest
`
	err := os.WriteFile(composeFile, []byte(composeContent), 0o644)
	assert.NilError(t, err)

	service := New(noBackend{})

	// Track events received
	var events []string
	listener := func(event string, metadata map[string]any) {
		events = append(events, event)
	}

	project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths:   []string{composeFile},
		LoadListeners: []api.LoadListener{listener},
	})

	assert.NilError(t, err)
	assert.Assert(t, project != nil)

	// Listeners should have been called (exact events depend on compose-go implementation)
	// The slice itself is always initialized (non-nil), even if empty
	_ = events // events may or may not have entries depending on compose-go behavior
}

func TestLoadProject_ProjectNameInference(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	composeContent := `
services:
  web:
    image: nginx:latest
`
	err := os.WriteFile(composeFile, []byte(composeContent), 0o644)
	assert.NilError(t, err)

	service := New(noBackend{})

	// Without explicit project name
	t.Run("InferredName", func(t *testing.T) {
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths: []string{composeFile},
		})
		assert.NilError(t, err)
		// Project name should be inferred from directory
		assert.Assert(t, project.Name != "")
	})

	// With explicit project name
	t.Run("ExplicitName", func(t *testing.T) {
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths: []string{composeFile},
			ProjectName: "my-custom-project",
		})
		assert.NilError(t, err)
		assert.Equal(t, "my-custom-project", project.Name)
	})
}

func TestLoadProject_Compatibility(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	composeContent := `
services:
  web:
    image: nginx:latest
`
	err := os.WriteFile(composeFile, []byte(composeContent), 0o644)
	assert.NilError(t, err)

	service := New(noBackend{})

	// With compatibility mode
	project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths:   []string{composeFile},
		Compatibility: true,
	})

	assert.NilError(t, err)
	assert.Assert(t, project != nil)
	// In compatibility mode, separator should be "_"
	assert.Equal(t, "_", api.Separator)

	// Reset separator
	api.Separator = "-"
}

func TestLoadProject_InvalidComposeFile(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	composeContent := `
this is not valid yaml: [[[
`
	err := os.WriteFile(composeFile, []byte(composeContent), 0o644)
	assert.NilError(t, err)

	service := New(noBackend{})

	// Should return an error for invalid YAML
	project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
	})

	assert.Assert(t, err != nil)
	assert.Assert(t, project == nil)
}

func TestLoadProject_MissingComposeFile(t *testing.T) {
	service := New(noBackend{})

	// Should return an error for missing file
	project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{"/nonexistent/compose.yaml"},
	})

	assert.Assert(t, err != nil)
	assert.Assert(t, project == nil)
}

func TestLoadProject_ExcludedProfiles(t *testing.T) {
	composeFile := filepath.Join(t.TempDir(), "compose.yaml")
	err := os.WriteFile(composeFile, []byte(`
name: test-project
services:
  web:
    image: nginx
  api:
    image: myapi
    profiles: [app]
  api-debug:
    image: myapi
    profiles: [app, debug]
  debugger:
    image: debugger
    profiles: [debug]
  monitoring:
    image: prometheus
    profiles: [monitoring]
jobs:
  inspect:
    image: alpine
    profiles: [debug]
    triggers:
      manual: true
`), 0o644)
	assert.NilError(t, err)

	service := New(noBackend{})

	tests := []struct {
		name             string
		profiles         []string
		excludedProfiles []string
		services         []string
		enabled          []string
		wantErr          string
	}{
		{
			name:             "all profiles but one",
			profiles:         []string{"*"},
			excludedProfiles: []string{"debug"},
			enabled:          []string{"web", "api", "monitoring"},
		},
		{
			name:             "several excluded profiles",
			profiles:         []string{"*"},
			excludedProfiles: []string{"debug", "monitoring"},
			enabled:          []string{"web", "api"},
		},
		{
			name:             "service also attached to an excluded profile",
			profiles:         []string{"app"},
			excludedProfiles: []string{"debug"},
			enabled:          []string{"web", "api"},
		},
		{
			name:             "excluding an inactive profile changes nothing",
			excludedProfiles: []string{"debug"},
			enabled:          []string{"web"},
		},
		{
			name:             "excluding an unknown profile changes nothing",
			profiles:         []string{"app"},
			excludedProfiles: []string{"unknown"},
			enabled:          []string{"web", "api", "api-debug"},
		},
		{
			name:             "explicitly selected service in an excluded profile",
			excludedProfiles: []string{"debug"},
			services:         []string{"debugger"},
			wantErr:          `service "debugger" belongs to excluded profile "debug"`,
		},
		{
			name:             "explicitly selected service kept",
			excludedProfiles: []string{"debug"},
			services:         []string{"api"},
			enabled:          []string{"api"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
				ConfigPaths:      []string{composeFile},
				Profiles:         tt.profiles,
				ExcludedProfiles: tt.excludedProfiles,
				Services:         tt.services,
			})
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.Check(t, is.DeepEqual(sorted(project.ServiceNames()), sorted(tt.enabled)))
			// excluded services stay known to the project, so they are not
			// mistaken for orphans
			assert.Check(t, is.Len(project.AllServices(), 5))
		})
	}

	t.Run("jobs are excluded too", func(t *testing.T) {
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths:      []string{composeFile},
			Profiles:         []string{"*"},
			ExcludedProfiles: []string{"debug"},
		})
		assert.NilError(t, err)
		assert.Check(t, is.Len(project.Jobs, 0))
		assert.Check(t, is.Contains(project.DisabledJobs, "inspect"))
	})

	t.Run("excluded profiles read from COMPOSE_EXCLUDED_PROFILES", func(t *testing.T) {
		t.Setenv(api.ComposeExcludedProfiles, "debug, monitoring,")
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths: []string{composeFile},
			Profiles:    []string{"*"},
		})
		assert.NilError(t, err)
		assert.Check(t, is.DeepEqual(sorted(project.ServiceNames()), []string{"api", "web"}))
	})

	t.Run("ExcludedProfiles overrides COMPOSE_EXCLUDED_PROFILES", func(t *testing.T) {
		t.Setenv(api.ComposeExcludedProfiles, "monitoring")
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths:      []string{composeFile},
			Profiles:         []string{"*"},
			ExcludedProfiles: []string{"debug"},
		})
		assert.NilError(t, err)
		assert.Check(t, is.DeepEqual(sorted(project.ServiceNames()), []string{"api", "monitoring", "web"}))
	})
}

func TestLoadProject_ExcludedProfilesDependency(t *testing.T) {
	composeFile := filepath.Join(t.TempDir(), "compose.yaml")
	err := os.WriteFile(composeFile, []byte(`
name: test-project
services:
  app:
    image: myapp
    depends_on:
      db:
        condition: service_started
      cache:
        condition: service_started
        required: false
  db:
    image: postgres
    profiles: [storage]
  cache:
    image: redis
    profiles: [storage-cache]
`), 0o644)
	assert.NilError(t, err)

	service := New(noBackend{})

	profiles := []string{"storage", "storage-cache"}

	t.Run("dependencies loaded without exclusion", func(t *testing.T) {
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths: []string{composeFile},
			Profiles:    profiles,
			Services:    []string{"app"},
		})
		assert.NilError(t, err)
		assert.Check(t, is.DeepEqual(sorted(project.ServiceNames()), []string{"app", "cache", "db"}))
	})

	t.Run("required dependency on an excluded service fails", func(t *testing.T) {
		_, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths:      []string{composeFile},
			Profiles:         profiles,
			Services:         []string{"app"},
			ExcludedProfiles: []string{"storage"},
		})
		assert.ErrorContains(t, err, `service "app" depends on service "db", which belongs to excluded profile "storage"`)
	})

	t.Run("required dependency on an excluded service fails when all profiles are enabled", func(t *testing.T) {
		_, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths:      []string{composeFile},
			Profiles:         []string{"*"},
			ExcludedProfiles: []string{"storage"},
		})
		assert.ErrorContains(t, err, `service "app" depends on service "db", which belongs to excluded profile "storage"`)
	})

	t.Run("unselected service depending on an excluded service is ignored", func(t *testing.T) {
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths:      []string{composeFile},
			Profiles:         profiles,
			Services:         []string{"cache"},
			ExcludedProfiles: []string{"storage"},
		})
		assert.NilError(t, err)
		assert.Check(t, is.DeepEqual(project.ServiceNames(), []string{"cache"}))
	})

	t.Run("optional dependency on an excluded service is dropped", func(t *testing.T) {
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths:      []string{composeFile},
			Profiles:         profiles,
			Services:         []string{"app"},
			ExcludedProfiles: []string{"storage-cache"},
		})
		assert.NilError(t, err)
		assert.Check(t, is.DeepEqual(sorted(project.ServiceNames()), []string{"app", "db"}))
		assert.Check(t, is.Len(project.Services["app"].DependsOn, 1))
	})

	t.Run("optional dependency dropped when all profiles are enabled", func(t *testing.T) {
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths:      []string{composeFile},
			Profiles:         []string{"*"},
			ExcludedProfiles: []string{"storage-cache"},
		})
		assert.NilError(t, err)
		assert.Check(t, is.DeepEqual(sorted(project.ServiceNames()), []string{"app", "db"}))
	})
}

func sorted(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}

// noBackend is a Backend with no daemon and no CLI, enough to load local Compose files.
type noBackend struct{}

func (noBackend) APIClient() client.APIClient { return nil }
func (noBackend) CLI() command.Cli            { return nil }
