package amazon

import (
	"fmt"
	"testing"

	"github.com/docker/ecs-plugin/pkg/compose"
	"gotest.tools/v3/golden"
)

func TestSimpleConvert(t *testing.T) {
	options := compose.ProjectOptions{
		Name:        t.Name(),
		ConfigPaths: []string{"testdata/input/simple-single-service.yaml"},
	}
	result := convertResultAsString(t, options, "TestCluster")
	expected := "simple/simple-cloudformation-conversion.golden"
	golden.Assert(t, result, expected)
}

func TestSimpleWithOverrides(t *testing.T) {
	options := compose.ProjectOptions{
		Name:        t.Name(),
		ConfigPaths: []string{"testdata/input/simple-single-service.yaml", "testdata/input/simple-single-service-with-overrides.yaml"},
	}
	result := convertResultAsString(t, options, "TestCluster")
	expected := "simple/simple-cloudformation-with-overrides-conversion.golden"
	golden.Assert(t, result, expected)
}

func convertResultAsString(t *testing.T, options compose.ProjectOptions, clusterName string) string {
	project, err := compose.ProjectFromOptions(&options)
	if err != nil {
		t.Error(err)
	}
	client, err := NewClient("", clusterName, "")
	if err != nil {
		t.Error(err)
	}
	result, err := client.Convert(project)
	if err != nil {
		t.Error(err)
	}
	resultAsJSON, err := result.JSON()
	if err != nil {
		t.Error(err)
	}
	return fmt.Sprintf("%s\n", string(resultAsJSON))
}
