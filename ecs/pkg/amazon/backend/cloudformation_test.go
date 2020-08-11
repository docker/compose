package backend

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/ec2"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/awslabs/goformation/v4/cloudformation/elasticloadbalancingv2"
	"github.com/awslabs/goformation/v4/cloudformation/iam"
	"github.com/awslabs/goformation/v4/cloudformation/logs"
	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/ecs-plugin/pkg/compose"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/golden"
)

func TestSimpleConvert(t *testing.T) {
	project := load(t, "testdata/input/simple-single-service.yaml")
	result := convertResultAsString(t, project)
	expected := "simple/simple-cloudformation-conversion.golden"
	golden.Assert(t, result, expected)
}

func TestLogging(t *testing.T) {
	template := convertYaml(t, "test", `
services:
  foo:
    image: hello_world
    logging:
      options:
        awslogs-datetime-pattern: "FOO"

x-aws-logs_retention: 10
`)
	def := template.Resources["FooTaskDefinition"].(*ecs.TaskDefinition)
	logging := def.ContainerDefinitions[0].LogConfiguration
	assert.Equal(t, logging.Options["awslogs-datetime-pattern"], "FOO")

	logGroup := template.Resources["LogGroup"].(*logs.LogGroup)
	assert.Equal(t, logGroup.RetentionInDays, 10)
}

func TestEnvFile(t *testing.T) {
	template := convertYaml(t, "test", `
services:
  foo:
    image: hello_world
    env_file:
      - testdata/input/envfile
`)
	def := template.Resources["FooTaskDefinition"].(*ecs.TaskDefinition)
	env := def.ContainerDefinitions[0].Environment
	var found bool
	for _, pair := range env {
		if pair.Name == "FOO" {
			assert.Equal(t, pair.Value, "BAR")
			found = true
		}
	}
	assert.Check(t, found, "environment variable FOO not set")

}

func TestEnvFileAndEnv(t *testing.T) {
	template := convertYaml(t, "test", `
services:
  foo:
    image: hello_world
    env_file:
      - testdata/input/envfile
    environment:
      - "FOO=ZOT"
`)
	def := template.Resources["FooTaskDefinition"].(*ecs.TaskDefinition)
	env := def.ContainerDefinitions[0].Environment
	assert.Equal(t, env[0].Name, "FOO")
	assert.Equal(t, env[0].Value, "ZOT")
}

func TestRollingUpdateLimits(t *testing.T) {
	template := convertYaml(t, "test", `
services:
  foo:
    image: hello_world
    deploy:
      replicas: 4 
      update_config:
        parallelism: 2
`)
	service := template.Resources["FooService"].(*ecs.Service)
	assert.Check(t, service.DeploymentConfiguration.MaximumPercent == 150)
	assert.Check(t, service.DeploymentConfiguration.MinimumHealthyPercent == 50)
}

func TestRollingUpdateExtension(t *testing.T) {
	template := convertYaml(t, "test", `
services:
  foo:
    image: hello_world
    deploy:
      update_config:
        x-aws-min_percent: 25
        x-aws-max_percent: 125
`)
	service := template.Resources["FooService"].(*ecs.Service)
	assert.Check(t, service.DeploymentConfiguration.MaximumPercent == 125)
	assert.Check(t, service.DeploymentConfiguration.MinimumHealthyPercent == 25)
}

func TestRolePolicy(t *testing.T) {
	template := convertYaml(t, "test", `
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
	template := convertYaml(t, "test", `
services:
  test:
    image: hello_world
    networks:
      - front-tier
      - back-tier

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
	template := convertYaml(t, "test123456789009876543211234567890", `
services:
  test:
    image: nginx
    ports:
      - 80:80
`)
	lb := template.Resources["TestLoadBalancer"].(*elasticloadbalancingv2.LoadBalancer)
	assert.Check(t, lb != nil)
	assert.Check(t, len(lb.Name) <= 32)
	assert.Check(t, lb.Type == elbv2.LoadBalancerTypeEnumApplication)
	assert.Check(t, len(lb.SecurityGroups) > 0)
}

func TestNoLoadBalancerIfNoPortExposed(t *testing.T) {
	template := convertYaml(t, "test", `
services:
  test:
    image: nginx
  foo:
    image: bar
`)
	for _, r := range template.Resources {
		assert.Check(t, r.AWSCloudFormationType() != "AWS::ElasticLoadBalancingV2::TargetGroup")
		assert.Check(t, r.AWSCloudFormationType() != "AWS::ElasticLoadBalancingV2::Listener")
		assert.Check(t, r.AWSCloudFormationType() != "AWS::ElasticLoadBalancingV2::LoadBalancer")
	}
}

func TestServiceReplicas(t *testing.T) {
	template := convertYaml(t, "test", `
services:
  test:
    image: nginx
    deploy:
      replicas: 10
`)
	s := template.Resources["TestService"].(*ecs.Service)
	assert.Check(t, s != nil)
	assert.Check(t, s.DesiredCount == 10)
}

func TestTaskSizeConvert(t *testing.T) {
	template := convertYaml(t, "test", `
services:
  test:
    image: nginx
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 2048M
        reservations:
          cpus: '0.5'
          memory: 2048M
`)
	def := template.Resources["TestTaskDefinition"].(*ecs.TaskDefinition)
	assert.Equal(t, def.Cpu, "512")
	assert.Equal(t, def.Memory, "2048")

	template = convertYaml(t, "test", `
services:
  test:
    image: nginx
    deploy:
      resources:
        limits:
          cpus: '4'
          memory: 8192M
        reservations:
          cpus: '4'
          memory: 8192M
`)
	def = template.Resources["TestTaskDefinition"].(*ecs.TaskDefinition)
	assert.Equal(t, def.Cpu, "4096")
	assert.Equal(t, def.Memory, "8192")
}
func TestTaskSizeConvertFailure(t *testing.T) {
	model := loadConfig(t, "test", `
services:
  test:
    image: nginx
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 2043248M
`)
	_, err := Backend{}.Convert(model)
	assert.ErrorContains(t, err, "the resources requested are not supported by ECS/Fargate")
}

func TestLoadBalancerTypeNetwork(t *testing.T) {
	template := convertYaml(t, "test", `
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

func TestServiceMapping(t *testing.T) {
	template := convertYaml(t, "test", `
services:
  test:
    image: "image"
    command: "command"
    entrypoint: "entrypoint"
    environment:
      - "FOO=BAR"
    cap_add:
      - SYS_PTRACE
    cap_drop:
      - SYSLOG
    init: true
    user: "user"
    working_dir: "working_dir"
`)
	def := template.Resources["TestTaskDefinition"].(*ecs.TaskDefinition)
	container := def.ContainerDefinitions[0]
	assert.Equal(t, container.Image, "image")
	assert.Equal(t, container.Command[0], "command")
	assert.Equal(t, container.EntryPoint[0], "entrypoint")
	assert.Equal(t, get(container.Environment, "FOO"), "BAR")
	assert.Check(t, container.LinuxParameters.InitProcessEnabled)
	assert.Equal(t, container.LinuxParameters.Capabilities.Add[0], "SYS_PTRACE")
	assert.Equal(t, container.LinuxParameters.Capabilities.Drop[0], "SYSLOG")
	assert.Equal(t, container.User, "user")
	assert.Equal(t, container.WorkingDirectory, "working_dir")
}

func get(l []ecs.TaskDefinition_KeyValuePair, name string) string {
	for _, e := range l {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

func TestResourcesHaveProjectTagSet(t *testing.T) {
	template := convertYaml(t, "test", `
services:
  test:
    image: nginx
    ports:
      - 80:80
      - 88:88
`)
	for _, r := range template.Resources {
		tags := reflect.Indirect(reflect.ValueOf(r)).FieldByName("Tags")
		if !tags.IsValid() {
			continue
		}
		for i := 0; i < tags.Len(); i++ {
			k := tags.Index(i).FieldByName("Key").String()
			v := tags.Index(i).FieldByName("Value").String()
			if k == compose.ProjectTag {
				assert.Equal(t, v, "Test")
			}
		}
	}
}

func convertResultAsString(t *testing.T, project *types.Project) string {
	backend, err := NewBackend("", "")
	assert.NilError(t, err)
	result, err := backend.Convert(project)
	assert.NilError(t, err)
	resultAsJSON, err := result.JSON()
	assert.NilError(t, err)
	return fmt.Sprintf("%s\n", string(resultAsJSON))
}

func load(t *testing.T, paths ...string) *types.Project {
	options := cli.ProjectOptions{
		Name:        t.Name(),
		ConfigPaths: paths,
	}
	project, err := cli.ProjectFromOptions(&options)
	assert.NilError(t, err)
	return project
}

func convertYaml(t *testing.T, name string, yaml string) *cloudformation.Template {
	model := loadConfig(t, name, yaml)
	template, err := Backend{}.Convert(model)
	assert.NilError(t, err)
	return template
}

func loadConfig(t *testing.T, name string, yaml string) *types.Project {
	dict, err := loader.ParseYAML([]byte(yaml))
	assert.NilError(t, err)
	model, err := loader.Load(types.ConfigDetails{
		ConfigFiles: []types.ConfigFile{
			{Config: dict},
		},
	}, func(options *loader.Options) {
		options.Name = "Test"
	})
	assert.NilError(t, err)
	return model
}
