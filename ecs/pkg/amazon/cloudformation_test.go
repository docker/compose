package amazon

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/ec2"
	"github.com/awslabs/goformation/v4/cloudformation/iam"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/elasticloadbalancingv2"
	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/ecs-plugin/pkg/compose"
	"gotest.tools/assert"
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

func TestRolePolicy(t *testing.T) {
	template := convertYaml(t, `
version: "3"
services:
  foo:
    image: hello_world
    x-aws-pull_credentials: "secret"
`)
	role := template.Resources["FooTaskExecutionRole"].(*iam.Role)
	assert.Check(t, role != nil)
	assert.Check(t, role.ManagedPolicyArns[0] == ECSTaskExecutionPolicy)
	assert.Check(t, role.ManagedPolicyArns[1] == ECRReadOnlyPolicy)
	// We expect an extra policy has been created for x-aws-pull_credentials
	assert.Check(t, len(role.Policies) == 1)
	policy := role.Policies[0].PolicyDocument.(*PolicyDocument)
	expected := []string{"secretsmanager:GetSecretValue", "ssm:GetParameters", "kms:Decrypt"}
	assert.DeepEqual(t, expected, policy.Statement[0].Action)
	assert.DeepEqual(t, []string{"secret"}, policy.Statement[0].Resource)
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

func TestLoadBalancerTypeApplication(t *testing.T) {
	template := convertYaml(t, `
version: "3"
services:
  test:
    image: nginx
    ports:
      - 80:80
`)
	lb := template.Resources["TestLoadBalancer"].(*elasticloadbalancingv2.LoadBalancer)
	assert.Check(t, lb != nil)
	assert.Check(t, lb.Type == elbv2.LoadBalancerTypeEnumApplication)
	assert.Check(t, len(lb.SecurityGroups) > 0)
}

func TestLoadBalancerTypeNetwork(t *testing.T) {
	template := convertYaml(t, `
version: "3"
services:
  test:
    image: nginx
    ports:
      - 80:80
      - 88:88
`)
	lb := template.Resources["TestLoadBalancer"].(*elasticloadbalancingv2.LoadBalancer)
	assert.Check(t, lb != nil)
	assert.Check(t, lb.Type == elbv2.LoadBalancerTypeEnumNetwork)
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
