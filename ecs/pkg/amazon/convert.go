package amazon

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	ecsapi "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/awslabs/goformation/v4/cloudformation/tags"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/opts"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func Convert(project *compose.Project, service types.ServiceConfig) (*ecs.TaskDefinition, error) {
	cpu, mem, err := toLimits(service)
	if err != nil {
		return nil, err
	}
	credential := getRepoCredentials(service)

	// override resolve.conf search directive to also search <project>.local
	// TODO remove once ECS support hostname-only service discovery
	service.Environment["LOCALDOMAIN"] = aws.String(
		cloudformation.Join("", []string{
			cloudformation.Ref("AWS::Region"),
			".compute.internal",
			fmt.Sprintf(" %s.local", project.Name),
		}))

	return &ecs.TaskDefinition{
		ContainerDefinitions: []ecs.TaskDefinition_ContainerDefinition{
			{
				Command:               service.Command,
				DisableNetworking:     service.NetworkMode == "none",
				DnsSearchDomains:      service.DNSSearch,
				DnsServers:            service.DNS,
				DockerLabels:          nil,
				DockerSecurityOptions: service.SecurityOpt,
				EntryPoint:            service.Entrypoint,
				Environment:           toKeyValuePair(service.Environment),
				Essential:             true,
				ExtraHosts:            toHostEntryPtr(service.ExtraHosts),
				FirelensConfiguration: nil,
				HealthCheck:           toHealthCheck(service.HealthCheck),
				Hostname:              service.Hostname,
				Image:                 getImage(service.Image),
				Interactive:           false,
				Links:                 nil,
				LinuxParameters:       toLinuxParameters(service),
				LogConfiguration: &ecs.TaskDefinition_LogConfiguration{
					LogDriver: ecsapi.LogDriverAwslogs,
					Options: map[string]string{
						"awslogs-region":        cloudformation.Ref("AWS::Region"),
						"awslogs-group":         cloudformation.Ref("LogGroup"),
						"awslogs-stream-prefix": project.Name,
					},
				},
				Name:                   service.Name,
				PortMappings:           toPortMappings(service.Ports),
				Privileged:             service.Privileged,
				PseudoTerminal:         service.Tty,
				ReadonlyRootFilesystem: service.ReadOnly,
				RepositoryCredentials:  credential,
				ResourceRequirements:   nil,
				StartTimeout:           0,
				StopTimeout:            durationToInt(service.StopGracePeriod),
				SystemControls:         toSystemControls(service.Sysctls),
				Ulimits:                toUlimits(service.Ulimits),
				User:                   service.User,
				VolumesFrom:            nil,
				WorkingDirectory:       service.WorkingDir,
			},
		},
		Cpu:                     cpu,
		Family:                  fmt.Sprintf("%s-%s", project.Name, service.Name),
		IpcMode:                 service.Ipc,
		Memory:                  mem,
		NetworkMode:             ecsapi.NetworkModeAwsvpc, // FIXME could be set by service.NetworkMode, Fargate only supports network mode ‘awsvpc’.
		PidMode:                 service.Pid,
		PlacementConstraints:    toPlacementConstraints(service.Deploy),
		ProxyConfiguration:      nil,
		RequiresCompatibilities: []string{ecsapi.LaunchTypeFargate},
		Tags:                    toTags(service.Labels),
	}, nil
}

func toTags(labels types.Labels) []tags.Tag {
	t := []tags.Tag{}
	for n, v := range labels {
		t = append(t, tags.Tag{
			Key:   n,
			Value: v,
		})
	}
	return t
}

func toSystemControls(sysctls types.Mapping) []ecs.TaskDefinition_SystemControl {
	sys := []ecs.TaskDefinition_SystemControl{}
	for k, v := range sysctls {
		sys = append(sys, ecs.TaskDefinition_SystemControl{
			Namespace: k,
			Value:     v,
		})
	}
	return sys
}

func toLimits(service types.ServiceConfig) (string, string, error) {
	// All possible cpu/mem values for Fargate
	cpuToMem := map[int64][]types.UnitBytes{
		256:  {512, 1024, 2048},
		512:  {1024, 2048, 3072, 4096},
		1024: {2048, 3072, 4096, 5120, 6144, 7168, 8192},
		2048: {4096, 5120, 6144, 7168, 8192, 9216, 10240, 11264, 12288, 13312, 14336, 15360, 16384},
		4096: {8192, 9216, 10240, 11264, 12288, 13312, 14336, 15360, 16384, 17408, 18432, 19456, 20480, 21504, 22528, 23552, 24576, 25600, 26624, 27648, 28672, 29696, 30720},
	}
	cpuLimit := "256"
	memLimit := "512"

	if service.Deploy == nil {
		return cpuLimit, memLimit, nil
	}

	limits := service.Deploy.Resources.Limits
	if limits == nil {
		return cpuLimit, memLimit, nil
	}

	if limits.NanoCPUs == "" {
		return cpuLimit, memLimit, nil
	}

	v, err := opts.ParseCPUs(limits.NanoCPUs)
	if err != nil {
		return "", "", err
	}

	for cpu, mem := range cpuToMem {
		if v <= cpu*1024*1024 {
			for _, m := range mem {
				if limits.MemoryBytes <= m*1024*1024 {
					cpuLimit = strconv.FormatInt(cpu, 10)
					memLimit = strconv.FormatInt(int64(m), 10)
					return cpuLimit, memLimit, nil
				}
			}
		}
	}
	return "", "", fmt.Errorf("unable to find cpu/mem for the required resources")
}

func toRequiresCompatibilities(isolation string) []*string {
	if isolation == "" {
		return nil
	}
	return []*string{&isolation}
}

func toPlacementConstraints(deploy *types.DeployConfig) []ecs.TaskDefinition_TaskDefinitionPlacementConstraint {
	if deploy == nil || deploy.Placement.Constraints == nil || len(deploy.Placement.Constraints) == 0 {
		return nil
	}
	pl := []ecs.TaskDefinition_TaskDefinitionPlacementConstraint{}
	for _, c := range deploy.Placement.Constraints {
		pl = append(pl, ecs.TaskDefinition_TaskDefinitionPlacementConstraint{
			Expression: c,
			Type:       "",
		})
	}
	return pl
}

func toPortMappings(ports []types.ServicePortConfig) []ecs.TaskDefinition_PortMapping {
	if len(ports) == 0 {
		return nil
	}
	m := []ecs.TaskDefinition_PortMapping{}
	for _, p := range ports {
		m = append(m, ecs.TaskDefinition_PortMapping{
			ContainerPort: int(p.Target),
			HostPort:      int(p.Published),
			Protocol:      p.Protocol,
		})
	}
	return m
}

func toUlimits(ulimits map[string]*types.UlimitsConfig) []ecs.TaskDefinition_Ulimit {
	if len(ulimits) == 0 {
		return nil
	}
	u := []ecs.TaskDefinition_Ulimit{}
	for k, v := range ulimits {
		u = append(u, ecs.TaskDefinition_Ulimit{
			Name:      k,
			SoftLimit: v.Soft,
			HardLimit: v.Hard,
		})
	}
	return u
}

const Mb = 1024 * 1024

func toLinuxParameters(service types.ServiceConfig) *ecs.TaskDefinition_LinuxParameters {
	return &ecs.TaskDefinition_LinuxParameters{
		Capabilities:       toKernelCapabilities(service.CapAdd, service.CapDrop),
		Devices:            nil,
		InitProcessEnabled: service.Init != nil && *service.Init,
		MaxSwap:            0,
		// FIXME SharedMemorySize:   service.ShmSize,
		Swappiness: 0,
		Tmpfs:      toTmpfs(service.Tmpfs),
	}
}

func toTmpfs(tmpfs types.StringList) []ecs.TaskDefinition_Tmpfs {
	if tmpfs == nil || len(tmpfs) == 0 {
		return nil
	}
	o := []ecs.TaskDefinition_Tmpfs{}
	for _, path := range tmpfs {
		o = append(o, ecs.TaskDefinition_Tmpfs{
			ContainerPath: path,
			Size:          100, // size is required on ECS, unlimited by the compose spec
		})
	}
	return o
}

func toKernelCapabilities(add []string, drop []string) *ecs.TaskDefinition_KernelCapabilities {
	if len(add) == 0 && len(drop) == 0 {
		return nil
	}
	return &ecs.TaskDefinition_KernelCapabilities{
		Add:  add,
		Drop: drop,
	}

}

func toHealthCheck(check *types.HealthCheckConfig) *ecs.TaskDefinition_HealthCheck {
	if check == nil {
		return nil
	}
	retries := 0
	if check.Retries != nil {
		retries = int(*check.Retries)
	}
	return &ecs.TaskDefinition_HealthCheck{
		Command:     check.Test,
		Interval:    durationToInt(check.Interval),
		Retries:     retries,
		StartPeriod: durationToInt(check.StartPeriod),
		Timeout:     durationToInt(check.Timeout),
	}
}

func durationToInt(interval *types.Duration) int {
	if interval == nil {
		return 0
	}
	v := int(time.Duration(*interval).Seconds())
	return v
}

func toHostEntryPtr(hosts types.HostsList) []ecs.TaskDefinition_HostEntry {
	if hosts == nil || len(hosts) == 0 {
		return nil
	}
	e := []ecs.TaskDefinition_HostEntry{}
	for _, h := range hosts {
		parts := strings.SplitN(h, ":", 2) // FIXME this should be handled by compose-go
		e = append(e, ecs.TaskDefinition_HostEntry{
			Hostname:  parts[0],
			IpAddress: parts[1],
		})
	}
	return e
}

func toKeyValuePair(environment types.MappingWithEquals) []ecs.TaskDefinition_KeyValuePair {
	if environment == nil || len(environment) == 0 {
		return nil
	}
	pairs := []ecs.TaskDefinition_KeyValuePair{}
	for k, v := range environment {
		name := k
		var value string
		if v != nil {
			value = *v
		}
		pairs = append(pairs, ecs.TaskDefinition_KeyValuePair{
			Name:  name,
			Value: value,
		})
	}
	return pairs
}

func getImage(image string) string {
	switch f := strings.Split(image, "/"); len(f) {
	case 1:
		return "docker.io/library/" + image
	case 2:
		return "docker.io/" + image
	default:
		return image
	}
}

func getRepoCredentials(service types.ServiceConfig) *ecs.TaskDefinition_RepositoryCredentials {
	// extract registry and namespace string from image name
	for key, value := range service.Extras {
		if key == ExtensionPullCredentials {
			return &ecs.TaskDefinition_RepositoryCredentials{CredentialsParameter: value.(string)}
		}
	}
	return nil
}
