package amazon

import (
	"fmt"
	"testing"

	"github.com/awslabs/goformation/v4/cloudformation/ec2"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"

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

func TestMapNetworksToSecurityGroups(t *testing.T) {
	template := convertYaml(t, `
version: "3"
services:
  test:
    image: hello_world
networks:
  front-tier:
    name: public
  back-tier:
    internal: true
`)
	assert.Check(t, template.Resources["TestPublicNetwork"] != nil)
	assert.Check(t, template.Resources["TestBacktierNetwork"] != nil)
	assert.Check(t, template.Resources["TestBacktierNetworkIngress"] != nil)
	ingress := template.Resources["TestPublicNetworkIngress"].(*ec2.SecurityGroupIngress)
	assert.Check(t, ingress != nil)
	assert.Check(t, ingress.SourceSecurityGroupId == cloudformation.Ref("TestPublicNetwork"))

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

func convertYaml(t *testing.T, yaml string) *cloudformation.Template {
	dict, err := loader.ParseYAML([]byte(yaml))
	assert.NilError(t, err)
	model, err := loader.Load(types.ConfigDetails{
		ConfigFiles: []types.ConfigFile{
			{Config: dict},
		},
	})
	assert.NilError(t, err)
	err = compose.Normalize(model)
	assert.NilError(t, err)
	template, err := client{}.Convert(&compose.Project{
		Config: *model,
		Name:   "test",
	})
	assert.NilError(t, err)
	return template
}
