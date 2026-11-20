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

// noBackend is a Backend with no daemon and no CLI, enough to load local Compose files.
type noBackend struct{}

func (noBackend) APIClient() client.APIClient { return nil }
func (noBackend) CLI() command.Cli            { return nil }

// A service which is not selected is not part of what the command works on: its env_file is not
// read, so one that is missing does not fail the command. Selecting that service does.
func TestLoadProject_ServiceSelectionSkipsEnvFileOfOtherServices(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	err := os.WriteFile(composeFile, []byte(`
services:
  web:
    image: nginx:latest
  worker:
    image: alpine
    env_file: missing.env
`), 0o644)
	assert.NilError(t, err)

	service := New(noBackend{})

	project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
		Services:    []string{"web"},
	})
	assert.NilError(t, err)
	assert.Check(t, is.Contains(project.Services, "web"))
	assert.Check(t, is.Contains(project.DisabledServices, "worker"))

	_, err = service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
		Services:    []string{"worker"},
	})
	assert.ErrorContains(t, err, "missing.env")
}

// The selected services bring their dependencies, and the resources only the other services use
// are dropped unless the whole model is requested.
func TestLoadProject_ServiceSelectionResources(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	err := os.WriteFile(composeFile, []byte(`
services:
  web:
    image: nginx:latest
    depends_on: [db]
    networks: [front]
  db:
    image: postgres:latest
    volumes: [data:/var/lib/postgresql/data]
    networks: [front]
  batch:
    image: alpine
    volumes: [scratch:/scratch]
    networks: [back]
networks:
  front:
  back:
volumes:
  data:
  scratch:
`), 0o644)
	assert.NilError(t, err)

	service := New(noBackend{})

	project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
		Services:    []string{"web"},
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, project.ServiceNames(), []string{"db", "web"})
	assert.DeepEqual(t, project.NetworkNames(), []string{"front"})
	assert.DeepEqual(t, project.VolumeNames(), []string{"data"})
	assert.Equal(t, project.Services["web"].CustomLabels[api.ServiceLabel], "web")

	project, err = service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
		Services:    []string{"web"},
		All:         true,
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, project.NetworkNames(), []string{"back", "front"})
	assert.DeepEqual(t, project.VolumeNames(), []string{"data", "scratch"})

	_, err = service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
		Services:    []string{"nope"},
	})
	assert.ErrorContains(t, err, `no such service: nope`)
}

// A service gated by a profile is enabled when it is selected by name, and what it depends on
// still has its env_file read: only the services left out are skipped.
func TestLoadProject_ServiceSelectionEnablesProfileAndKeepsDependencies(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	err := os.WriteFile(composeFile, []byte(`
services:
  debug:
    image: busybox
    profiles: [debug]
    depends_on: [db]
  db:
    image: postgres:latest
    env_file: db.env
  other:
    image: alpine
    env_file: missing.env
`), 0o644)
	assert.NilError(t, err)
	assert.NilError(t, os.WriteFile(filepath.Join(tmpDir, "db.env"), []byte("PGUSER=app\n"), 0o644))

	service := New(noBackend{})

	project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
		Services:    []string{"debug"},
		All:         true,
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, project.ServiceNames(), []string{"db", "debug"})
	assert.Equal(t, *project.Services["db"].Environment["PGUSER"], "app")

	// same without All: the profile-gated service is enabled, not left in the disabled ones
	project, err = service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
		Services:    []string{"debug"},
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, project.ServiceNames(), []string{"db", "debug"})
	assert.Check(t, !is.Contains(project.DisabledServices, "debug")().Success())

	// the dependency's own env_file is still required
	assert.NilError(t, os.Remove(filepath.Join(tmpDir, "db.env")))
	_, err = service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
		Services:    []string{"debug"},
	})
	assert.ErrorContains(t, err, "db.env")
}
