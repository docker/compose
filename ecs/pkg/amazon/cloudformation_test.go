package amazon

import (
	"fmt"
	"testing"

	"gotest.tools/assert"

	"github.com/docker/ecs-plugin/pkg/compose"
	"gotest.tools/v3/golden"
)

func TestSimpleConvert(t *testing.T) {
	project := load(t, "testdata/input/simple-single-service.yaml")
	result := convertResultAsString(t, project, "TestCluster")
	expected := "simple/simple-cloudformation-conversion.golden"
	golden.Assert(t, result, expected)
}

func TestSimpleWithOverrides(t *testing.T) {
	project := load(t, "testdata/input/simple-single-service.yaml", "testdata/input/simple-single-service-with-overrides.yaml")
	result := convertResultAsString(t, project, "TestCluster")
	expected := "simple/simple-cloudformation-with-overrides-conversion.golden"
	golden.Assert(t, result, expected)
}

func convertResultAsString(t *testing.T, project *compose.Project, clusterName string) string {
	client, err := NewClient("", clusterName, "")
	assert.NilError(t, err)
	result, err := client.Convert(project)
	assert.NilError(t, err)
	resultAsJSON, err := result.JSON()
	assert.NilError(t, err)
	return fmt.Sprintf("%s\n", string(resultAsJSON))
}

func load(t *testing.T, paths ...string) *compose.Project {
	options := compose.ProjectOptions{
		Name:        t.Name(),
		ConfigPaths: paths,
	}
	project, err := compose.ProjectFromOptions(&options)
	assert.NilError(t, err)
	return project
}
