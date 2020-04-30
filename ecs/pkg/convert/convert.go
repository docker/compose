package convert

import (
	"strings"
	"time"

	ecsapi "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/opts"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func Convert(project *compose.Project, service types.ServiceConfig) (*ecs.TaskDefinition, error) {
	_, err := toCPULimits(service)
	if err != nil {
		return nil, err
	}

	return &ecs.TaskDefinition{
		ContainerDefinitions: []ecs.TaskDefinition_ContainerDefinition{
			// Here we can declare sidecars and init-containers using https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task_definition_parameters.html#container_definition_dependson
			{
				Command:                service.Command,
				Cpu:                    256,
				DisableNetworking:      service.NetworkMode == "none",
				DnsSearchDomains:       service.DNSSearch,
				DnsServers:             service.DNS,
				DockerLabels:           nil,
				DockerSecurityOptions:  service.SecurityOpt,
				EntryPoint:             service.Entrypoint,
				Environment:            toKeyValuePair(service.Environment),
				Essential:              true,
				ExtraHosts:             toHostEntryPtr(service.ExtraHosts),
				FirelensConfiguration:  nil,
				HealthCheck:            toHealthCheck(service.HealthCheck),
				Hostname:               service.Hostname,
				Image:                  service.Image,
				Interactive:            false,
				Links:                  nil,
				LinuxParameters:        toLinuxParameters(service),
				Memory:                 toMemoryLimits(service.Deploy),
				MemoryReservation:      toMemoryReservation(service.Deploy),
				MountPoints:            nil,
				Name:                   service.Name,
				PortMappings:           toPortMappings(service.Ports),
				Privileged:             service.Privileged,
				PseudoTerminal:         service.Tty,
				ReadonlyRootFilesystem: service.ReadOnly,
				RepositoryCredentials:  nil,
				ResourceRequirements:   nil,
				Secrets:                nil,
				StartTimeout:           0,
				StopTimeout:            durationToInt(service.StopGracePeriod),
				SystemControls:         nil,
				Ulimits:                toUlimits(service.Ulimits),
				User:                   service.User,
				VolumesFrom:            nil,
				WorkingDirectory:       service.WorkingDir,
			},
		},
		Cpu:                     toCPU(service),
		Family:                  project.Name,
		IpcMode:                 service.Ipc,
		Memory:                  toMemory(service),
		NetworkMode:             ecsapi.NetworkModeAwsvpc, // FIXME could be set by service.NetworkMode, Fargate only supports network mode ‘awsvpc’.
		PidMode:                 service.Pid,
		PlacementConstraints:    toPlacementConstraints(service.Deploy),
		ProxyConfiguration:      nil,
		RequiresCompatibilities: []string{ecsapi.LaunchTypeFargate},
		Tags:                    nil,
		Volumes:                 []ecs.TaskDefinition_Volume{},
	}, nil
}

func toCPU(service types.ServiceConfig) string {
	// FIXME based on service's memory/cpu requirements, select the adequate Fargate CPU
	return "256"
}

func toMemory(service types.ServiceConfig) string {
	// FIXME based on service's memory/cpu requirements, select the adequate Fargate CPU
	return "512"
}

func toCPULimits(service types.ServiceConfig) (*int64, error) {
	if service.Deploy == nil {
		return nil, nil
	}
	res := service.Deploy.Resources.Limits
	if res == nil {
		return nil, nil
	}
	if res.NanoCPUs == "" {
		return nil, nil
	}
	v, err := opts.ParseCPUs(res.NanoCPUs)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func toRequiresCompatibilities(isolation string) []*string {
	if isolation == "" {
		return nil
	}
	return []*string{&isolation}
}

func hasMemoryOrMemoryReservation(service types.ServiceConfig) bool {
	if service.Deploy == nil {
		return false
	}
	if service.Deploy.Resources.Reservations != nil {
		return true
	}
	if service.Deploy.Resources.Limits != nil {
		return true
	}
	return false
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

func uint32Toint64Ptr(i uint32) *int64 {
	v := int64(i)
	return &v
}

func intToInt64Ptr(i int) *int64 {
	v := int64(i)
	return &v
}

const Mb = 1024 * 1024

func toMemoryLimits(deploy *types.DeployConfig) int {
	if deploy == nil {
		return 0
	}
	res := deploy.Resources.Limits
	if res == nil {
		return 0
	}
	v := int(res.MemoryBytes) / Mb
	return v
}

func toMemoryReservation(deploy *types.DeployConfig) int {
	if deploy == nil {
		return 0
	}
	res := deploy.Resources.Reservations
	if res == nil {
		return 0
	}
	v := int(res.MemoryBytes) / Mb
	return v
}

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
			MountOptions:  nil,
			Size:          0,
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

func uint64ToInt64Ptr(i *uint64) *int64 {
	if i == nil {
		return nil
	}
	v := int64(*i)
	return &v
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
