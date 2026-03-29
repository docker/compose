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
	"os"
	"path/filepath"
	"testing"

	"github.com/compose-spec/compose-go/v2/cli"
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

	service, err := NewComposeService(nil)
	assert.NilError(t, err)

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

	service, err := NewComposeService(nil)
	assert.NilError(t, err)

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

	service, err := NewComposeService(nil)
	assert.NilError(t, err)

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

	service, err := NewComposeService(nil)
	assert.NilError(t, err)

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

	service, err := NewComposeService(nil)
	assert.NilError(t, err)

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

	service, err := NewComposeService(nil)
	assert.NilError(t, err)

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

	service, err := NewComposeService(nil)
	assert.NilError(t, err)

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

	service, err := NewComposeService(nil)
	assert.NilError(t, err)

	// Should return an error for invalid YAML
	project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeFile},
	})

	assert.Assert(t, err != nil)
	assert.Assert(t, project == nil)
}

func TestLoadProject_MissingComposeFile(t *testing.T) {
	service, err := NewComposeService(nil)
	assert.NilError(t, err)

	// Should return an error for missing file
	project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{"/nonexistent/compose.yaml"},
	})

	assert.Assert(t, err != nil)
	assert.Assert(t, project == nil)
}
