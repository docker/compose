package convert

import (
	"github.com/docker/ecs-plugin/pkg/compose"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/opts"
)

func Convert(project *compose.Project, service types.ServiceConfig) (*ecs.RegisterTaskDefinitionInput, error) {
	_, err := toCPULimits(service)
	if err != nil {
		return nil, err
	}

	foo := int64(256)
	logDriver := "awslogs" // FIXME could be set by service.Logging, especially to enable use of firelens
	return &ecs.RegisterTaskDefinitionInput{
		ContainerDefinitions: []*ecs.ContainerDefinition{
			// Here we can declare sidecars and init-containers using https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task_definition_parameters.html#container_definition_dependson
			{
				Command:               toStringPtrSlice(service.Command),
				Cpu:                   &foo,
				DependsOn:             nil,
				DisableNetworking:     toBoolPtr(service.NetworkMode == "none"),
				DnsSearchDomains:      toStringPtrSlice(service.DNSSearch),
				DnsServers:            toStringPtrSlice(service.DNS),
				DockerLabels:          nil,
				DockerSecurityOptions: toStringPtrSlice(service.SecurityOpt),
				EntryPoint:            toStringPtrSlice(service.Entrypoint),
				Environment:           toKeyValuePairPtr(service.Environment),
				Essential:             toBoolPtr(true),
				ExtraHosts:            toHostEntryPtr(service.ExtraHosts),
				FirelensConfiguration: nil,
				HealthCheck:           toHealthCheck(service.HealthCheck),
				Hostname:              toStringPtr(service.Hostname),
				Image:                 toStringPtr(service.Image),
				Interactive:           nil,
				Links:                 nil,
				LinuxParameters:       toLinuxParameters(service),
				LogConfiguration: &ecs.LogConfiguration{
					LogDriver:     &logDriver,
					Options:       map[string]*string{},
					SecretOptions: nil,
				},
				Memory:                 toMemoryLimits(service.Deploy),
				MemoryReservation:      toMemoryReservation(service.Deploy),
				MountPoints:            nil,
				Name:                   toStringPtr(service.Name),
				PortMappings:           toPortMappings(service.Ports),
				Privileged:             toBoolPtr(service.Privileged),
				PseudoTerminal:         toBoolPtr(service.Tty),
				ReadonlyRootFilesystem: toBoolPtr(service.ReadOnly),
				RepositoryCredentials:  nil,
				ResourceRequirements:   nil,
				Secrets:                nil,
				StartTimeout:           nil,
				StopTimeout:            durationToInt64Ptr(service.StopGracePeriod),
				SystemControls:         nil,
				Ulimits:                toUlimits(service.Ulimits),
				User:                   toStringPtr(service.User),
				VolumesFrom:            nil,
				WorkingDirectory:       toStringPtr(service.WorkingDir),
			},
		},
		Cpu:                     toCPU(service),
		ExecutionRoleArn:        nil,
		Family:                  toStringPtr(project.Name),
		IpcMode:                 toStringPtr(service.Ipc),
		Memory:                  toMemory(service),
		NetworkMode:             toStringPtr("awsvpc"), // FIXME could be set by service.NetworkMode, Fargate only supports network mode ‘awsvpc’.
		PidMode:                 toStringPtr(service.Pid),
		PlacementConstraints:    toPlacementConstraints(service.Deploy),
		ProxyConfiguration:      nil,
		RequiresCompatibilities: toRequiresCompatibilities(ecs.LaunchTypeFargate),
		Tags:                    nil,
		Volumes: []*ecs.Volume{
			{
				/* ONLY supported when using EC2 launch type
				DockerVolumeConfiguration: {
					Autoprovision: nil,
					Driver:        nil,
					DriverOpts:    nil,
					Labels:        nil,
					Scope:         nil,
				}, */
				/* Beta and ONLY supported when using EC2 launch type
				EfsVolumeConfiguration: {
					FileSystemId:  nil,
					RootDirectory: nil,
				}, */
				/* Bind mount host volume
				Host:                      {
						SourcePath:
				}, */
				Name: aws.String("MyVolume"),
			},
		},
	}, nil

}

func toCPU(service types.ServiceConfig) *string {
	// FIXME based on service's memory/cpu requirements, select the adequate Fargate CPU
	v := "256"
	return &v
}

func toMemory(service types.ServiceConfig) *string {
	// FIXME based on service's memory/cpu requirements, select the adequate Fargate CPU
	v := "512"
	return &v
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

func toPlacementConstraints(deploy *types.DeployConfig) []*ecs.TaskDefinitionPlacementConstraint {
	if deploy == nil || deploy.Placement.Constraints == nil || len(deploy.Placement.Constraints) == 0 {
		return nil
	}
	pl := []*ecs.TaskDefinitionPlacementConstraint{}
	for _, c := range deploy.Placement.Constraints {
		pl = append(pl, &ecs.TaskDefinitionPlacementConstraint{
			Expression: toStringPtr(c),
			Type:       nil,
		})
	}
	return pl
}

func toPortMappings(ports []types.ServicePortConfig) []*ecs.PortMapping {
	if len(ports) == 0 {
		return nil
	}
	m := []*ecs.PortMapping{}
	for _, p := range ports {
		m = append(m, &ecs.PortMapping{
			ContainerPort: uint32Toint64Ptr(p.Target),
			HostPort:      uint32Toint64Ptr(p.Published),
			Protocol:      toStringPtr(p.Protocol),
		})
	}
	return m
}

func toUlimits(ulimits map[string]*types.UlimitsConfig) []*ecs.Ulimit {
	if len(ulimits) == 0 {
		return nil
	}
	u := []*ecs.Ulimit{}
	for k, v := range ulimits {
		u = append(u, &ecs.Ulimit{
			Name:      toStringPtr(k),
			SoftLimit: intToInt64Ptr(v.Soft),
			HardLimit: intToInt64Ptr(v.Hard),
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

func toMemoryLimits(deploy *types.DeployConfig) *int64 {
	if deploy == nil {
		return nil
	}
	res := deploy.Resources.Limits
	if res == nil {
		return nil
	}
	v := int64(res.MemoryBytes) / Mb
	return &v
}

func toMemoryReservation(deploy *types.DeployConfig) *int64 {
	if deploy == nil {
		return nil
	}
	res := deploy.Resources.Reservations
	if res == nil {
		return nil
	}
	v := int64(res.MemoryBytes) / Mb
	return &v
}

func toLinuxParameters(service types.ServiceConfig) *ecs.LinuxParameters {
	return &ecs.LinuxParameters{
		Capabilities:       toKernelCapabilities(service.CapAdd, service.CapDrop),
		Devices:            nil,
		InitProcessEnabled: service.Init,
		MaxSwap:            nil,
		// FIXME SharedMemorySize:   service.ShmSize,
		Swappiness: nil,
		Tmpfs:      toTmpfs(service.Tmpfs),
	}
}

func toTmpfs(tmpfs types.StringList) []*ecs.Tmpfs {
	if tmpfs == nil || len(tmpfs) == 0 {
		return nil
	}
	o := []*ecs.Tmpfs{}
	for _, t := range tmpfs {
		path := t
		o = append(o, &ecs.Tmpfs{
			ContainerPath: &path,
			MountOptions:  nil,
			Size:          nil,
		})
	}
	return o
}

func toKernelCapabilities(add []string, drop []string) *ecs.KernelCapabilities {
	if len(add) == 0 && len(drop) == 0 {
		return nil
	}
	return &ecs.KernelCapabilities{
		Add:  toStringPtrSlice(add),
		Drop: toStringPtrSlice(drop),
	}

}

func toHealthCheck(check *types.HealthCheckConfig) *ecs.HealthCheck {
	if check == nil {
		return nil
	}
	return &ecs.HealthCheck{
		Command:     toStringPtrSlice(check.Test),
		Interval:    durationToInt64Ptr(check.Interval),
		Retries:     uint64ToInt64Ptr(check.Retries),
		StartPeriod: durationToInt64Ptr(check.StartPeriod),
		Timeout:     durationToInt64Ptr(check.Timeout),
	}
}

func uint64ToInt64Ptr(i *uint64) *int64 {
	if i == nil {
		return nil
	}
	v := int64(*i)
	return &v
}

func durationToInt64Ptr(interval *types.Duration) *int64 {
	if interval == nil {
		return nil
	}
	v := int64(time.Duration(*interval).Seconds())
	return &v
}

func toHostEntryPtr(hosts types.HostsList) []*ecs.HostEntry {
	if hosts == nil || len(hosts) == 0 {
		return nil
	}
	e := []*ecs.HostEntry{}
	for _, h := range hosts {
		host := h
		e = append(e, &ecs.HostEntry{
			Hostname: &host,
		})
	}
	return e
}

func toKeyValuePairPtr(environment types.MappingWithEquals) []*ecs.KeyValuePair {
	if environment == nil || len(environment) == 0 {
		return nil
	}
	pairs := []*ecs.KeyValuePair{}
	for k, v := range environment {
		name := k
		value := v
		pairs = append(pairs, &ecs.KeyValuePair{
			Name:  &name,
			Value: value,
		})
	}
	return pairs
}

func toStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toStringPtrSlice(s []string) []*string {
	if len(s) == 0 {
		return nil
	}
	v := []*string{}
	for _, x := range s {
		value := x
		v = append(v, &value)
	}
	return v
}

func toBoolPtr(b bool) *bool {
	if !b {
		return nil
	}
	return &b
}