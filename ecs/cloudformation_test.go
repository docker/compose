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

package ecs

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/docker/compose-cli/api/compose"

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
	template := convertYaml(t, `
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
	template := convertYaml(t, `
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
	template := convertYaml(t, `
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
	var found bool
	for _, pair := range env {
		if pair.Name == "FOO" {
			assert.Equal(t, pair.Value, "ZOT")
			found = true
		}
	}
	assert.Check(t, found, "environment variable FOO not set")
}

func TestRollingUpdateLimits(t *testing.T) {
	template := convertYaml(t, `
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
	template := convertYaml(t, `
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
	template := convertYaml(t, `
services:
  foo:
    image: hello_world
    x-aws-pull_credentials: "secret"
`)
	x := template.Resources["FooTaskExecutionRole"]
	assert.Check(t, x != nil)
	role := *(x.(*iam.Role))
	assert.Check(t, role.ManagedPolicyArns[0] == ecsTaskExecutionPolicy)
	assert.Check(t, role.ManagedPolicyArns[1] == ecrReadOnlyPolicy)
	// We expect an extra policy has been created for x-aws-pull_credentials
	assert.Check(t, len(role.Policies) == 1)
	policy := role.Policies[0].PolicyDocument.(*PolicyDocument)
	expected := []string{"secretsmanager:GetSecretValue", "ssm:GetParameters", "kms:Decrypt"}
	assert.DeepEqual(t, expected, policy.Statement[0].Action)
	assert.DeepEqual(t, []string{"secret"}, policy.Statement[0].Resource)
}

func TestMapNetworksToSecurityGroups(t *testing.T) {
	template := convertYaml(t, `
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
	assert.Check(t, template.Resources["FronttierNetwork"] != nil)
	assert.Check(t, template.Resources["BacktierNetwork"] != nil)
	assert.Check(t, template.Resources["BacktierNetworkIngress"] != nil)
	i := template.Resources["FronttierNetworkIngress"]
	assert.Check(t, i != nil)
	ingress := *i.(*ec2.SecurityGroupIngress)
	assert.Check(t, ingress.SourceSecurityGroupId == cloudformation.Ref("FronttierNetwork"))

}

func TestLoadBalancerTypeApplication(t *testing.T) {
	cases := []string{
		`services:
  test:
    image: nginx
    ports:
      - 80:80
`,
		`services:
  test:
    image: nginx
    ports:
      - target: 8080
        x-aws-protocol: http
`,
	}
	for _, y := range cases {
		template := convertYaml(t, y)
		lb := template.Resources["LoadBalancer"]
		assert.Check(t, lb != nil)
		loadBalancer := *lb.(*elasticloadbalancingv2.LoadBalancer)
		assert.Check(t, len(loadBalancer.Name) <= 32)
		assert.Check(t, loadBalancer.Type == elbv2.LoadBalancerTypeEnumApplication)
		assert.Check(t, len(loadBalancer.SecurityGroups) > 0)
	}
}

func TestNoLoadBalancerIfNoPortExposed(t *testing.T) {
	template := convertYaml(t, `
services:
  test:
    image: nginx
  foo:
    image: bar
`)
	for _, r := range template.Resources {
		assert.Check(t, r.AWSCloudFormationType() != "AWS::ElasticLoadBalancingV2::TargetGroup")
		assert.Check(t, r.AWSCloudFormationType() != "AWS::ElasticLoadBalancingV2::Listener")
		assert.Check(t, r.AWSCloudFormationType() != "AWS::ElasticLoadBalancingV2::PortPublisher")
	}
}

func TestServiceReplicas(t *testing.T) {
	template := convertYaml(t, `
services:
  test:
    image: nginx
    deploy:
      replicas: 10
`)
	s := template.Resources["TestService"]
	assert.Check(t, s != nil)
	service := *s.(*ecs.Service)
	assert.Check(t, service.DesiredCount == 10)
}

func TestTaskSizeConvert(t *testing.T) {
	template := convertYaml(t, `
services:
  test:
    image: nginx
`)
	def := template.Resources["TestTaskDefinition"].(*ecs.TaskDefinition)
	assert.Equal(t, def.Cpu, "256")
	assert.Equal(t, def.Memory, "512")

	template = convertYaml(t, `
services:
  test:
    image: nginx
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 2048M
`)
	def = template.Resources["TestTaskDefinition"].(*ecs.TaskDefinition)
	assert.Equal(t, def.Cpu, "512")
	assert.Equal(t, def.Memory, "2048")

	template = convertYaml(t, `
services:
  test:
    image: nginx
    deploy:
      resources:
        limits:
          cpus: '4'
          memory: 8192M
`)
	def = template.Resources["TestTaskDefinition"].(*ecs.TaskDefinition)
	assert.Equal(t, def.Cpu, "4096")
	assert.Equal(t, def.Memory, "8192")

	template = convertYaml(t, `
services:
  test:
    image: nginx
    deploy:
      resources:
        limits:
          cpus: '4'
          memory: 792Mb
        reservations:
          generic_resources: 
            - discrete_resource_spec:
                kind: gpus
                value: 2
`)
	def = template.Resources["TestTaskDefinition"].(*ecs.TaskDefinition)
	assert.Equal(t, def.Cpu, "4000")
	assert.Equal(t, def.Memory, "792")

	template = convertYaml(t, `
services:
  test:
    image: nginx
    deploy:
      resources:
        reservations:
          generic_resources: 
            - discrete_resource_spec:
                kind: gpus
                value: 2
`)
	def = template.Resources["TestTaskDefinition"].(*ecs.TaskDefinition)
	assert.Equal(t, def.Cpu, "")
	assert.Equal(t, def.Memory, "")
}
func TestTaskSizeConvertFailure(t *testing.T) {
	model := loadConfig(t, `
services:
  test:
    image: nginx
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 2043248M
`)
	backend := &ecsAPIService{}
	_, err := backend.convert(model)
	assert.ErrorContains(t, err, "the resources requested are not supported by ECS/Fargate")
}

func TestLoadBalancerTypeNetwork(t *testing.T) {
	template := convertYaml(t, `
services:
  test:
    image: nginx
    ports:
      - 80:80
      - 88:88
`)
	lb := template.Resources["LoadBalancer"]
	assert.Check(t, lb != nil)
	loadBalancer := *lb.(*elasticloadbalancingv2.LoadBalancer)
	assert.Check(t, loadBalancer.Type == elbv2.LoadBalancerTypeEnumNetwork)
}

func TestServiceMapping(t *testing.T) {
	template := convertYaml(t, `
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
	template := convertYaml(t, `
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
	backend := &ecsAPIService{
		resources: awsResources{
			vpc:     "vpcID",
			subnets: []string{"subnet1", "subnet2"},
		},
	}
	template, err := backend.convert(project)
	assert.NilError(t, err)
	resultAsJSON, err := marshall(template)
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

func convertYaml(t *testing.T, yaml string) *cloudformation.Template {
	project := loadConfig(t, yaml)
	backend := &ecsAPIService{}
	template, err := backend.convert(project)
	assert.NilError(t, err)
	return template
}

func loadConfig(t *testing.T, yaml string) *types.Project {
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
