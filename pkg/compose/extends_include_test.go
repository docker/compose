/*
   Test to reproduce issue #13606:
   service.extends doesn't resolve includes properly

   When a service extends another service that's defined in an included file,
   environment variables from the base service's includes aren't propagated
   to the extending service.
*/

package compose

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/compose/v5/pkg/api"
)

func TestExtendsWithInclude_InheritsEnvironment(t *testing.T) {
	// Create temporary directory for test files
	tmpDir := t.TempDir()

	// Create compose.a.yml with a service that has environment variable A
	composeAFile := filepath.Join(tmpDir, "compose.a.yml")
	composeAContent := `services:
  web:
    image: scratch
    environment:
      A: "A"
`
	err := os.WriteFile(composeAFile, []byte(composeAContent), 0o644)
	require.NoError(t, err)

	// Create compose.b.yml that includes compose.a.yml
	// and has a worker service that extends web
	composeBFile := filepath.Join(tmpDir, "compose.b.yml")
	composeBContent := `include:
  - compose.a.yml
services:
  web:
    environment:
      B: B

  worker:
    image: scratch
    extends:
      service: web
    environment:
      C: C
`
	err = os.WriteFile(composeBFile, []byte(composeBContent), 0o644)
	require.NoError(t, err)

	// Load the project
	service, err := NewComposeService(nil)
	require.NoError(t, err)

	project, err := service.LoadProject(context.Background(), api.ProjectLoadOptions{
		ConfigPaths: []string{composeBFile},
	})
	require.NoError(t, err)

	// Verify web service has both A and B environment variables
	webService := project.Services["web"]
	assert.Contains(t, webService.Environment, "A", "web service should have A from include")
	assert.Contains(t, webService.Environment, "B", "web service should have B")
	if webService.Environment["A"] != nil {
		assert.Equal(t, "A", *webService.Environment["A"])
	}
	if webService.Environment["B"] != nil {
		assert.Equal(t, "B", *webService.Environment["B"])
	}

	// THIS IS THE BUG: worker should have A, B, and C
	// But currently it only has B and C (missing A from the included file)
	workerService := project.Services["worker"]
	assert.Contains(t, workerService.Environment, "A", "worker should inherit A from web (via include)")
	assert.Contains(t, workerService.Environment, "B", "worker should inherit B from web")
	assert.Contains(t, workerService.Environment, "C", "worker should have C")
	if workerService.Environment["A"] != nil {
		assert.Equal(t, "A", *workerService.Environment["A"])
	}
	if workerService.Environment["B"] != nil {
		assert.Equal(t, "B", *workerService.Environment["B"])
	}
	if workerService.Environment["C"] != nil {
		assert.Equal(t, "C", *workerService.Environment["C"])
	}
}
