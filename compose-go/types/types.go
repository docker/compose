/*
   Copyright 2020 The Compose Specification Authors.

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

package types

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
)

// Duration is a thin wrapper around time.Duration with improved JSON marshalling
type Duration time.Duration

func (d Duration) String() string {
	return time.Duration(d).String()
}

// ConvertDurationPtr converts a type defined Duration pointer to a time.Duration pointer with the same value.
func ConvertDurationPtr(d *Duration) *time.Duration {
	if d == nil {
		return nil
	}
	res := time.Duration(*d)
	return &res
}

// MarshalJSON makes Duration implement json.Marshaler
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// MarshalYAML makes Duration implement yaml.Marshaler
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), "\"")
	timeDuration, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(timeDuration)
	return nil
}

// Services is a list of ServiceConfig
type Services []ServiceConfig

// MarshalYAML makes Services implement yaml.Marshaller
func (s Services) MarshalYAML() (interface{}, error) {
	services := map[string]ServiceConfig{}
	for _, service := range s {
		services[service.Name] = service
	}
	return services, nil
}

// MarshalJSON makes Services implement json.Marshaler
func (s Services) MarshalJSON() ([]byte, error) {
	data, err := s.MarshalYAML()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(data, "", "  ")
}

// ServiceConfig is the configuration of one service
type ServiceConfig struct {
	Name     string   `yaml:"-" json:"-"`
	Profiles []string `mapstructure:"profiles" yaml:"profiles,omitempty" json:"profiles,omitempty"`

	Build             *BuildConfig                     `yaml:",omitempty" json:"build,omitempty"`
	BlkioConfig       *BlkioConfig                     `mapstructure:"blkio_config" yaml:",omitempty" json:"blkio_config,omitempty"`
	CapAdd            []string                         `mapstructure:"cap_add" yaml:"cap_add,omitempty" json:"cap_add,omitempty"`
	CapDrop           []string                         `mapstructure:"cap_drop" yaml:"cap_drop,omitempty" json:"cap_drop,omitempty"`
	CgroupParent      string                           `mapstructure:"cgroup_parent" yaml:"cgroup_parent,omitempty" json:"cgroup_parent,omitempty"`
	CPUCount          int64                            `mapstructure:"cpu_count" yaml:"cpu_count,omitempty" json:"cpu_count,omitempty"`
	CPUPercent        float32                          `mapstructure:"cpu_percent" yaml:"cpu_percent,omitempty" json:"cpu_percent,omitempty"`
	CPUPeriod         int64                            `mapstructure:"cpu_period" yaml:"cpu_period,omitempty" json:"cpu_period,omitempty"`
	CPUQuota          int64                            `mapstructure:"cpu_quota" yaml:"cpu_quota,omitempty" json:"cpu_quota,omitempty"`
	CPURTPeriod       int64                            `mapstructure:"cpu_rt_period" yaml:"cpu_rt_period,omitempty" json:"cpu_rt_period,omitempty"`
	CPURTRuntime      int64                            `mapstructure:"cpu_rt_runtime" yaml:"cpu_rt_runtime,omitempty" json:"cpu_rt_runtime,omitempty"`
	CPUS              float32                          `mapstructure:"cpus" yaml:"cpus,omitempty" json:"cpus,omitempty"`
	CPUSet            string                           `mapstructure:"cpuset" yaml:"cpuset,omitempty" json:"cpuset,omitempty"`
	CPUShares         int64                            `mapstructure:"cpu_shares" yaml:"cpu_shares,omitempty" json:"cpu_shares,omitempty"`
	Command           ShellCommand                     `yaml:",omitempty" json:"command,omitempty"`
	Configs           []ServiceConfigObjConfig         `yaml:",omitempty" json:"configs,omitempty"`
	ContainerName     string                           `mapstructure:"container_name" yaml:"container_name,omitempty" json:"container_name,omitempty"`
	CredentialSpec    *CredentialSpecConfig            `mapstructure:"credential_spec" yaml:"credential_spec,omitempty" json:"credential_spec,omitempty"`
	DependsOn         DependsOnConfig                  `mapstructure:"depends_on" yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Deploy            *DeployConfig                    `yaml:",omitempty" json:"deploy,omitempty"`
	DeviceCgroupRules []string                         `mapstructure:"device_cgroup_rules" yaml:"device_cgroup_rules,omitempty" json:"device_cgroup_rules,omitempty"`
	Devices           []string                         `yaml:",omitempty" json:"devices,omitempty"`
	DNS               StringList                       `yaml:",omitempty" json:"dns,omitempty"`
	DNSOpts           []string                         `mapstructure:"dns_opt" yaml:"dns_opt,omitempty" json:"dns_opt,omitempty"`
	DNSSearch         StringList                       `mapstructure:"dns_search" yaml:"dns_search,omitempty" json:"dns_search,omitempty"`
	Dockerfile        string                           `yaml:"dockerfile,omitempty" json:"dockerfile,omitempty"`
	DomainName        string                           `mapstructure:"domainname" yaml:"domainname,omitempty" json:"domainname,omitempty"`
	Entrypoint        ShellCommand                     `yaml:",omitempty" json:"entrypoint,omitempty"`
	Environment       MappingWithEquals                `yaml:",omitempty" json:"environment,omitempty"`
	EnvFile           StringList                       `mapstructure:"env_file" yaml:"env_file,omitempty" json:"env_file,omitempty"`
	Expose            StringOrNumberList               `yaml:",omitempty" json:"expose,omitempty"`
	Extends           ExtendsConfig                    `yaml:"extends,omitempty" json:"extends,omitempty"`
	ExternalLinks     []string                         `mapstructure:"external_links" yaml:"external_links,omitempty" json:"external_links,omitempty"`
	ExtraHosts        HostsList                        `mapstructure:"extra_hosts" yaml:"extra_hosts,omitempty" json:"extra_hosts,omitempty"`
	GroupAdd          []string                         `mapstructure:"group_add" yaml:"group_add,omitempty" json:"group_add,omitempty"`
	Hostname          string                           `yaml:",omitempty" json:"hostname,omitempty"`
	HealthCheck       *HealthCheckConfig               `yaml:",omitempty" json:"healthcheck,omitempty"`
	Image             string                           `yaml:",omitempty" json:"image,omitempty"`
	Init              *bool                            `yaml:",omitempty" json:"init,omitempty"`
	Ipc               string                           `yaml:",omitempty" json:"ipc,omitempty"`
	Isolation         string                           `mapstructure:"isolation" yaml:"isolation,omitempty" json:"isolation,omitempty"`
	Labels            Labels                           `yaml:",omitempty" json:"labels,omitempty"`
	CustomLabels      Labels                           `yaml:"-" json:"-"`
	Links             []string                         `yaml:",omitempty" json:"links,omitempty"`
	Logging           *LoggingConfig                   `yaml:",omitempty" json:"logging,omitempty"`
	LogDriver         string                           `mapstructure:"log_driver" yaml:"log_driver,omitempty" json:"log_driver,omitempty"`
	LogOpt            map[string]string                `mapstructure:"log_opt" yaml:"log_opt,omitempty" json:"log_opt,omitempty"`
	MemLimit          UnitBytes                        `mapstructure:"mem_limit" yaml:"mem_limit,omitempty" json:"mem_limit,omitempty"`
	MemReservation    UnitBytes                        `mapstructure:"mem_reservation" yaml:"mem_reservation,omitempty" json:"mem_reservation,omitempty"`
	MemSwapLimit      UnitBytes                        `mapstructure:"memswap_limit" yaml:"memswap_limit,omitempty" json:"memswap_limit,omitempty"`
	MemSwappiness     UnitBytes                        `mapstructure:"mem_swappiness" yaml:"mem_swappiness,omitempty" json:"mem_swappiness,omitempty"`
	MacAddress        string                           `mapstructure:"mac_address" yaml:"mac_address,omitempty" json:"mac_address,omitempty"`
	Net               string                           `yaml:"net,omitempty" json:"net,omitempty"`
	NetworkMode       string                           `mapstructure:"network_mode" yaml:"network_mode,omitempty" json:"network_mode,omitempty"`
	Networks          map[string]*ServiceNetworkConfig `yaml:",omitempty" json:"networks,omitempty"`
	OomKillDisable    bool                             `mapstructure:"oom_kill_disable" yaml:"oom_kill_disable,omitempty" json:"oom_kill_disable,omitempty"`
	OomScoreAdj       int64                            `mapstructure:"oom_score_adj" yaml:"oom_score_adj,omitempty" json:"oom_score_adj,omitempty"`
	Pid               string                           `yaml:",omitempty" json:"pid,omitempty"`
	PidsLimit         int64                            `mapstructure:"pids_limit" yaml:"pids_limit,omitempty" json:"pids_limit,omitempty"`
	Platform          string                           `yaml:",omitempty" json:"platform,omitempty"`
	Ports             []ServicePortConfig              `yaml:",omitempty" json:"ports,omitempty"`
	Privileged        bool                             `yaml:",omitempty" json:"privileged,omitempty"`
	PullPolicy        string                           `mapstructure:"pull_policy" yaml:"pull_policy,omitempty" json:"pull_policy,omitempty"`
	ReadOnly          bool                             `mapstructure:"read_only" yaml:"read_only,omitempty" json:"read_only,omitempty"`
	Restart           string                           `yaml:",omitempty" json:"restart,omitempty"`
	Runtime           string                           `yaml:",omitempty" json:"runtime,omitempty"`
	Scale             int                              `yaml:"-" json:"-"`
	Secrets           []ServiceSecretConfig            `yaml:",omitempty" json:"secrets,omitempty"`
	SecurityOpt       []string                         `mapstructure:"security_opt" yaml:"security_opt,omitempty" json:"security_opt,omitempty"`
	ShmSize           UnitBytes                        `mapstructure:"shm_size" yaml:"shm_size,omitempty" json:"shm_size,omitempty"`
	StdinOpen         bool                             `mapstructure:"stdin_open" yaml:"stdin_open,omitempty" json:"stdin_open,omitempty"`
	StopGracePeriod   *Duration                        `mapstructure:"stop_grace_period" yaml:"stop_grace_period,omitempty" json:"stop_grace_period,omitempty"`
	StopSignal        string                           `mapstructure:"stop_signal" yaml:"stop_signal,omitempty" json:"stop_signal,omitempty"`
	Sysctls           Mapping                          `yaml:",omitempty" json:"sysctls,omitempty"`
	Tmpfs             StringList                       `yaml:",omitempty" json:"tmpfs,omitempty"`
	Tty               bool                             `mapstructure:"tty" yaml:"tty,omitempty" json:"tty,omitempty"`
	Ulimits           map[string]*UlimitsConfig        `yaml:",omitempty" json:"ulimits,omitempty"`
	User              string                           `yaml:",omitempty" json:"user,omitempty"`
	UserNSMode        string                           `mapstructure:"userns_mode" yaml:"userns_mode,omitempty" json:"userns_mode,omitempty"`
	Uts               string                           `yaml:"uts,omitempty" json:"uts,omitempty"`
	VolumeDriver      string                           `mapstructure:"volume_driver" yaml:"volume_driver,omitempty" json:"volume_driver,omitempty"`
	Volumes           []ServiceVolumeConfig            `yaml:",omitempty" json:"volumes,omitempty"`
	VolumesFrom       []string                         `mapstructure:"volumes_from" yaml:"volumes_from,omitempty" json:"volumes_from,omitempty"`
	WorkingDir        string                           `mapstructure:"working_dir" yaml:"working_dir,omitempty" json:"working_dir,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// NetworksByPriority return the service networks IDs sorted according to Priority
func (s *ServiceConfig) NetworksByPriority() []string {
	type key struct {
		name     string
		priority int
	}
	var keys []key
	for k, v := range s.Networks {
		priority := 0
		if v != nil {
			priority = v.Priority
		}
		keys = append(keys, key{
			name:     k,
			priority: priority,
		})
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].priority > keys[j].priority
	})
	var sorted []string
	for _, k := range keys {
		sorted = append(sorted, k.name)
	}
	return sorted
}

const (
	//PullPolicyAlways always pull images
	PullPolicyAlways = "always"
	//PullPolicyNever never pull images
	PullPolicyNever = "never"
	//PullPolicyIfNotPresent pull missing images
	PullPolicyIfNotPresent = "if_not_present"
	//PullPolicyMissing pull missing images
	PullPolicyMissing = "missing"
	//PullPolicyBuild force building images
	PullPolicyBuild = "build"
)

const (
	//RestartPolicyAlways always restart the container if it stops
	RestartPolicyAlways = "always"
	//RestartPolicyOnFailure restart the container if it exits due to an error
	RestartPolicyOnFailure = "on-failure"
	//RestartPolicyNo do not automatically restart the container
	RestartPolicyNo = "no"
	//RestartPolicyUnlessStopped always restart the container unless the container is stopped (manually or otherwise)
	RestartPolicyUnlessStopped = "unless-stopped"
)

const (
	// ServicePrefix is the prefix for references pointing to a service
	ServicePrefix = "service:"
	// ContainerPrefix is the prefix for references pointing to a container
	ContainerPrefix = "container:"

	// NetworkModeServicePrefix is the prefix for network_mode pointing to a service
	// Deprecated prefer ServicePrefix
	NetworkModeServicePrefix = ServicePrefix
	// NetworkModeContainerPrefix is the prefix for network_mode pointing to a container
	// Deprecated prefer ContainerPrefix
	NetworkModeContainerPrefix = ContainerPrefix
)

// GetDependencies retrieve all services this service depends on
func (s ServiceConfig) GetDependencies() []string {
	dependencies := make(set)
	for dependency := range s.DependsOn {
		dependencies.append(dependency)
	}
	for _, link := range s.Links {
		parts := strings.Split(link, ":")
		if len(parts) == 2 {
			dependencies.append(parts[0])
		} else {
			dependencies.append(link)
		}
	}
	if strings.HasPrefix(s.NetworkMode, ServicePrefix) {
		dependencies.append(s.NetworkMode[len(ServicePrefix):])
	}
	if strings.HasPrefix(s.Ipc, ServicePrefix) {
		dependencies.append(s.Ipc[len(ServicePrefix):])
	}
	if strings.HasPrefix(s.Pid, ServicePrefix) {
		dependencies.append(s.Pid[len(ServicePrefix):])
	}
	for _, vol := range s.VolumesFrom {
		if !strings.HasPrefix(s.Pid, ContainerPrefix) {
			dependencies.append(vol)
		}
	}

	return dependencies.toSlice()
}

type set map[string]struct{}

func (s set) append(strings ...string) {
	for _, str := range strings {
		s[str] = struct{}{}
	}
}

func (s set) toSlice() []string {
	slice := make([]string, 0, len(s))
	for v := range s {
		slice = append(slice, v)
	}
	return slice
}

// BuildConfig is a type for build
type BuildConfig struct {
	Context    string            `yaml:",omitempty" json:"context,omitempty"`
	Dockerfile string            `yaml:",omitempty" json:"dockerfile,omitempty"`
	Args       MappingWithEquals `yaml:",omitempty" json:"args,omitempty"`
	SSH        SSHConfig         `yaml:"ssh,omitempty" json:"ssh,omitempty"`
	Labels     Labels            `yaml:",omitempty" json:"labels,omitempty"`
	CacheFrom  StringList        `mapstructure:"cache_from" yaml:"cache_from,omitempty" json:"cache_from,omitempty"`
	CacheTo    StringList        `mapstructure:"cache_to" yaml:"cache_to,omitempty" json:"cache_to,omitempty"`
	NoCache    bool              `mapstructure:"no_cache" yaml:"no_cache,omitempty" json:"no_cache,omitempty"`
	Pull       bool              `mapstructure:"pull" yaml:"pull,omitempty" json:"pull,omitempty"`
	ExtraHosts HostsList         `mapstructure:"extra_hosts" yaml:"extra_hosts,omitempty" json:"extra_hosts,omitempty"`
	Isolation  string            `yaml:",omitempty" json:"isolation,omitempty"`
	Network    string            `yaml:",omitempty" json:"network,omitempty"`
	Target     string            `yaml:",omitempty" json:"target,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// BlkioConfig define blkio config
type BlkioConfig struct {
	Weight          uint16           `yaml:",omitempty" json:"weight,omitempty"`
	WeightDevice    []WeightDevice   `mapstructure:"weight_device" yaml:",omitempty" json:"weight_device,omitempty"`
	DeviceReadBps   []ThrottleDevice `mapstructure:"device_read_bps" yaml:",omitempty" json:"device_read_bps,omitempty"`
	DeviceReadIOps  []ThrottleDevice `mapstructure:"device_read_iops" yaml:",omitempty" json:"device_read_iops,omitempty"`
	DeviceWriteBps  []ThrottleDevice `mapstructure:"device_write_bps" yaml:",omitempty" json:"device_write_bps,omitempty"`
	DeviceWriteIOps []ThrottleDevice `mapstructure:"device_write_iops" yaml:",omitempty" json:"device_write_iops,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// WeightDevice is a structure that holds device:weight pair
type WeightDevice struct {
	Path   string
	Weight uint16

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// ThrottleDevice is a structure that holds device:rate_per_second pair
type ThrottleDevice struct {
	Path string
	Rate uint64

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// ShellCommand is a string or list of string args
type ShellCommand []string

// StringList is a type for fields that can be a string or list of strings
type StringList []string

// StringOrNumberList is a type for fields that can be a list of strings or
// numbers
type StringOrNumberList []string

// MappingWithEquals is a mapping type that can be converted from a list of
// key[=value] strings.
// For the key with an empty value (`key=`), the mapped value is set to a pointer to `""`.
// For the key without value (`key`), the mapped value is set to nil.
type MappingWithEquals map[string]*string

// NewMappingWithEquals build a new Mapping from a set of KEY=VALUE strings
func NewMappingWithEquals(values []string) MappingWithEquals {
	mapping := MappingWithEquals{}
	for _, env := range values {
		tokens := strings.SplitN(env, "=", 2)
		if len(tokens) > 1 {
			mapping[tokens[0]] = &tokens[1]
		} else {
			mapping[env] = nil
		}
	}
	return mapping
}

// OverrideBy update MappingWithEquals with values from another MappingWithEquals
func (e MappingWithEquals) OverrideBy(other MappingWithEquals) MappingWithEquals {
	for k, v := range other {
		e[k] = v
	}
	return e
}

// Resolve update a MappingWithEquals for keys without value (`key`, but not `key=`)
func (e MappingWithEquals) Resolve(lookupFn func(string) (string, bool)) MappingWithEquals {
	for k, v := range e {
		if v == nil {
			if value, ok := lookupFn(k); ok {
				e[k] = &value
			}
		}
	}
	return e
}

// RemoveEmpty excludes keys that are not associated with a value
func (e MappingWithEquals) RemoveEmpty() MappingWithEquals {
	for k, v := range e {
		if v == nil {
			delete(e, k)
		}
	}
	return e
}

// Mapping is a mapping type that can be converted from a list of
// key[=value] strings.
// For the key with an empty value (`key=`), or key without value (`key`), the
// mapped value is set to an empty string `""`.
type Mapping map[string]string

// NewMapping build a new Mapping from a set of KEY=VALUE strings
func NewMapping(values []string) Mapping {
	mapping := Mapping{}
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		key := parts[0]
		switch {
		case len(parts) == 1:
			mapping[key] = ""
		default:
			mapping[key] = parts[1]
		}
	}
	return mapping
}

// Labels is a mapping type for labels
type Labels map[string]string

func (l Labels) Add(key, value string) Labels {
	if l == nil {
		l = Labels{}
	}
	l[key] = value
	return l
}

type SSHKey struct {
	ID   string
	Path string
}

// SSHConfig is a mapping type for SSH build config
type SSHConfig []SSHKey

func (s SSHConfig) Get(id string) (string, error) {
	for _, sshKey := range s {
		if sshKey.ID == id {
			return sshKey.Path, nil
		}
	}
	return "", fmt.Errorf("ID %s not found in SSH keys", id)
}

// MarshalYAML makes SSHKey implement yaml.Marshaller
func (s SSHKey) MarshalYAML() (interface{}, error) {
	if s.Path == "" {
		return s.ID, nil
	}
	return fmt.Sprintf("%s: %s", s.ID, s.Path), nil
}

// MarshalJSON makes SSHKey implement json.Marshaller
func (s SSHKey) MarshalJSON() ([]byte, error) {
	if s.Path == "" {
		return []byte(fmt.Sprintf(`"%s"`, s.ID)), nil
	}
	return []byte(fmt.Sprintf(`"%s": %s`, s.ID, s.Path)), nil
}

// MappingWithColon is a mapping type that can be converted from a list of
// 'key: value' strings
type MappingWithColon map[string]string

// HostsList is a list of colon-separated host-ip mappings
type HostsList []string

// LoggingConfig the logging configuration for a service
type LoggingConfig struct {
	Driver  string            `yaml:",omitempty" json:"driver,omitempty"`
	Options map[string]string `yaml:",omitempty" json:"options,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// DeployConfig the deployment configuration for a service
type DeployConfig struct {
	Mode           string         `yaml:",omitempty" json:"mode,omitempty"`
	Replicas       *uint64        `yaml:",omitempty" json:"replicas,omitempty"`
	Labels         Labels         `yaml:",omitempty" json:"labels,omitempty"`
	UpdateConfig   *UpdateConfig  `mapstructure:"update_config" yaml:"update_config,omitempty" json:"update_config,omitempty"`
	RollbackConfig *UpdateConfig  `mapstructure:"rollback_config" yaml:"rollback_config,omitempty" json:"rollback_config,omitempty"`
	Resources      Resources      `yaml:",omitempty" json:"resources,omitempty"`
	RestartPolicy  *RestartPolicy `mapstructure:"restart_policy" yaml:"restart_policy,omitempty" json:"restart_policy,omitempty"`
	Placement      Placement      `yaml:",omitempty" json:"placement,omitempty"`
	EndpointMode   string         `mapstructure:"endpoint_mode" yaml:"endpoint_mode,omitempty" json:"endpoint_mode,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// HealthCheckConfig the healthcheck configuration for a service
type HealthCheckConfig struct {
	Test        HealthCheckTest `yaml:",omitempty" json:"test,omitempty"`
	Timeout     *Duration       `yaml:",omitempty" json:"timeout,omitempty"`
	Interval    *Duration       `yaml:",omitempty" json:"interval,omitempty"`
	Retries     *uint64         `yaml:",omitempty" json:"retries,omitempty"`
	StartPeriod *Duration       `mapstructure:"start_period" yaml:"start_period,omitempty" json:"start_period,omitempty"`
	Disable     bool            `yaml:",omitempty" json:"disable,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// HealthCheckTest is the command run to test the health of a service
type HealthCheckTest []string

// UpdateConfig the service update configuration
type UpdateConfig struct {
	Parallelism     *uint64  `yaml:",omitempty" json:"parallelism,omitempty"`
	Delay           Duration `yaml:",omitempty" json:"delay,omitempty"`
	FailureAction   string   `mapstructure:"failure_action" yaml:"failure_action,omitempty" json:"failure_action,omitempty"`
	Monitor         Duration `yaml:",omitempty" json:"monitor,omitempty"`
	MaxFailureRatio float32  `mapstructure:"max_failure_ratio" yaml:"max_failure_ratio,omitempty" json:"max_failure_ratio,omitempty"`
	Order           string   `yaml:",omitempty" json:"order,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// Resources the resource limits and reservations
type Resources struct {
	Limits       *Resource `yaml:",omitempty" json:"limits,omitempty"`
	Reservations *Resource `yaml:",omitempty" json:"reservations,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// Resource is a resource to be limited or reserved
type Resource struct {
	// TODO: types to convert from units and ratios
	NanoCPUs         string            `mapstructure:"cpus" yaml:"cpus,omitempty" json:"cpus,omitempty"`
	MemoryBytes      UnitBytes         `mapstructure:"memory" yaml:"memory,omitempty" json:"memory,omitempty"`
	PIds             int64             `mapstructure:"pids" yaml:"pids,omitempty" json:"pids,omitempty"`
	Devices          []DeviceRequest   `mapstructure:"devices" yaml:"devices,omitempty" json:"devices,omitempty"`
	GenericResources []GenericResource `mapstructure:"generic_resources" yaml:"generic_resources,omitempty" json:"generic_resources,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

type DeviceRequest struct {
	Capabilities []string `mapstructure:"capabilities" yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	Driver       string   `mapstructure:"driver" yaml:"driver,omitempty" json:"driver,omitempty"`
	Count        int64    `mapstructure:"count" yaml:"count,omitempty" json:"count,omitempty"`
	IDs          []string `mapstructure:"device_ids" yaml:"device_ids,omitempty" json:"device_ids,omitempty"`
}

// GenericResource represents a "user defined" resource which can
// only be an integer (e.g: SSD=3) for a service
type GenericResource struct {
	DiscreteResourceSpec *DiscreteGenericResource `mapstructure:"discrete_resource_spec" yaml:"discrete_resource_spec,omitempty" json:"discrete_resource_spec,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// DiscreteGenericResource represents a "user defined" resource which is defined
// as an integer
// "Kind" is used to describe the Kind of a resource (e.g: "GPU", "FPGA", "SSD", ...)
// Value is used to count the resource (SSD=5, HDD=3, ...)
type DiscreteGenericResource struct {
	Kind  string `json:"kind"`
	Value int64  `json:"value"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// UnitBytes is the bytes type
type UnitBytes int64

// MarshalYAML makes UnitBytes implement yaml.Marshaller
func (u UnitBytes) MarshalYAML() (interface{}, error) {
	return fmt.Sprintf("%d", u), nil
}

// MarshalJSON makes UnitBytes implement json.Marshaler
func (u UnitBytes) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%d"`, u)), nil
}

// RestartPolicy the service restart policy
type RestartPolicy struct {
	Condition   string    `yaml:",omitempty" json:"condition,omitempty"`
	Delay       *Duration `yaml:",omitempty" json:"delay,omitempty"`
	MaxAttempts *uint64   `mapstructure:"max_attempts" yaml:"max_attempts,omitempty" json:"max_attempts,omitempty"`
	Window      *Duration `yaml:",omitempty" json:"window,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// Placement constraints for the service
type Placement struct {
	Constraints []string               `yaml:",omitempty" json:"constraints,omitempty"`
	Preferences []PlacementPreferences `yaml:",omitempty" json:"preferences,omitempty"`
	MaxReplicas uint64                 `mapstructure:"max_replicas_per_node" yaml:"max_replicas_per_node,omitempty" json:"max_replicas_per_node,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// PlacementPreferences is the preferences for a service placement
type PlacementPreferences struct {
	Spread string `yaml:",omitempty" json:"spread,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// ServiceNetworkConfig is the network configuration for a service
type ServiceNetworkConfig struct {
	Priority    int      `yaml:",omitempty" json:"priotirt,omitempty"`
	Aliases     []string `yaml:",omitempty" json:"aliases,omitempty"`
	Ipv4Address string   `mapstructure:"ipv4_address" yaml:"ipv4_address,omitempty" json:"ipv4_address,omitempty"`
	Ipv6Address string   `mapstructure:"ipv6_address" yaml:"ipv6_address,omitempty" json:"ipv6_address,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// ServicePortConfig is the port configuration for a service
type ServicePortConfig struct {
	Mode      string `yaml:",omitempty" json:"mode,omitempty"`
	HostIP    string `mapstructure:"host_ip" yaml:"host_ip,omitempty" json:"host_ip,omitempty"`
	Target    uint32 `yaml:",omitempty" json:"target,omitempty"`
	Published string `yaml:",omitempty" json:"published,omitempty"`
	Protocol  string `yaml:",omitempty" json:"protocol,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// ParsePortConfig parse short syntax for service port configuration
func ParsePortConfig(value string) ([]ServicePortConfig, error) {
	var portConfigs []ServicePortConfig
	ports, portBindings, err := nat.ParsePortSpecs([]string{value})
	if err != nil {
		return nil, err
	}
	// We need to sort the key of the ports to make sure it is consistent
	keys := []string{}
	for port := range ports {
		keys = append(keys, string(port))
	}
	sort.Strings(keys)

	for _, key := range keys {
		port := nat.Port(key)
		converted, err := convertPortToPortConfig(port, portBindings)
		if err != nil {
			return nil, err
		}
		portConfigs = append(portConfigs, converted...)
	}
	return portConfigs, nil
}

func convertPortToPortConfig(port nat.Port, portBindings map[nat.Port][]nat.PortBinding) ([]ServicePortConfig, error) {
	var portConfigs []ServicePortConfig
	for _, binding := range portBindings[port] {
		portConfigs = append(portConfigs, ServicePortConfig{
			HostIP:    binding.HostIP,
			Protocol:  strings.ToLower(port.Proto()),
			Target:    uint32(port.Int()),
			Published: binding.HostPort,
			Mode:      "ingress",
		})
	}
	return portConfigs, nil
}

// ServiceVolumeConfig are references to a volume used by a service
type ServiceVolumeConfig struct {
	Type        string               `yaml:",omitempty" json:"type,omitempty"`
	Source      string               `yaml:",omitempty" json:"source,omitempty"`
	Target      string               `yaml:",omitempty" json:"target,omitempty"`
	ReadOnly    bool                 `mapstructure:"read_only" yaml:"read_only,omitempty" json:"read_only,omitempty"`
	Consistency string               `yaml:",omitempty" json:"consistency,omitempty"`
	Bind        *ServiceVolumeBind   `yaml:",omitempty" json:"bind,omitempty"`
	Volume      *ServiceVolumeVolume `yaml:",omitempty" json:"volume,omitempty"`
	Tmpfs       *ServiceVolumeTmpfs  `yaml:",omitempty" json:"tmpfs,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

const (
	// VolumeTypeBind is the type for mounting host dir
	VolumeTypeBind = "bind"
	// VolumeTypeVolume is the type for remote storage volumes
	VolumeTypeVolume = "volume"
	// VolumeTypeTmpfs is the type for mounting tmpfs
	VolumeTypeTmpfs = "tmpfs"
	// VolumeTypeNamedPipe is the type for mounting Windows named pipes
	VolumeTypeNamedPipe = "npipe"

	// SElinuxShared share the volume content
	SElinuxShared = "z"
	// SElinuxUnshared label content as private unshared
	SElinuxUnshared = "Z"
)

// ServiceVolumeBind are options for a service volume of type bind
type ServiceVolumeBind struct {
	SELinux        string `mapstructure:"selinux" yaml:",omitempty" json:"selinux,omitempty"`
	Propagation    string `yaml:",omitempty" json:"propagation,omitempty"`
	CreateHostPath bool   `mapstructure:"create_host_path" yaml:"create_host_path,omitempty" json:"create_host_path,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// SELinux represents the SELinux re-labeling options.
const (
	// SELinuxShared option indicates that the bind mount content is shared among multiple containers
	SELinuxShared string = "z"
	// SELinuxPrivate option indicates that the bind mount content is private and unshared
	SELinuxPrivate string = "Z"
)

// Propagation represents the propagation of a mount.
const (
	// PropagationRPrivate RPRIVATE
	PropagationRPrivate string = "rprivate"
	// PropagationPrivate PRIVATE
	PropagationPrivate string = "private"
	// PropagationRShared RSHARED
	PropagationRShared string = "rshared"
	// PropagationShared SHARED
	PropagationShared string = "shared"
	// PropagationRSlave RSLAVE
	PropagationRSlave string = "rslave"
	// PropagationSlave SLAVE
	PropagationSlave string = "slave"
)

// ServiceVolumeVolume are options for a service volume of type volume
type ServiceVolumeVolume struct {
	NoCopy bool `mapstructure:"nocopy" yaml:"nocopy,omitempty" json:"nocopy,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// ServiceVolumeTmpfs are options for a service volume of type tmpfs
type ServiceVolumeTmpfs struct {
	Size UnitBytes `yaml:",omitempty" json:"size,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// FileReferenceConfig for a reference to a swarm file object
type FileReferenceConfig struct {
	Source string  `yaml:",omitempty" json:"source,omitempty"`
	Target string  `yaml:",omitempty" json:"target,omitempty"`
	UID    string  `yaml:",omitempty" json:"uid,omitempty"`
	GID    string  `yaml:",omitempty" json:"gid,omitempty"`
	Mode   *uint32 `yaml:",omitempty" json:"mode,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// ServiceConfigObjConfig is the config obj configuration for a service
type ServiceConfigObjConfig FileReferenceConfig

// ServiceSecretConfig is the secret configuration for a service
type ServiceSecretConfig FileReferenceConfig

// UlimitsConfig the ulimit configuration
type UlimitsConfig struct {
	Single int `yaml:",omitempty" json:"single,omitempty"`
	Soft   int `yaml:",omitempty" json:"soft,omitempty"`
	Hard   int `yaml:",omitempty" json:"hard,omitempty"`

	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// MarshalYAML makes UlimitsConfig implement yaml.Marshaller
func (u *UlimitsConfig) MarshalYAML() (interface{}, error) {
	if u.Single != 0 {
		return u.Single, nil
	}
	return u, nil
}

// MarshalJSON makes UlimitsConfig implement json.Marshaller
func (u *UlimitsConfig) MarshalJSON() ([]byte, error) {
	if u.Single != 0 {
		return json.Marshal(u.Single)
	}
	// Pass as a value to avoid re-entering this method and use the default implementation
	return json.Marshal(*u)
}

// NetworkConfig for a network
type NetworkConfig struct {
	Name       string                 `yaml:",omitempty" json:"name,omitempty"`
	Driver     string                 `yaml:",omitempty" json:"driver,omitempty"`
	DriverOpts map[string]string      `mapstructure:"driver_opts" yaml:"driver_opts,omitempty" json:"driver_opts,omitempty"`
	Ipam       IPAMConfig             `yaml:",omitempty" json:"ipam,omitempty"`
	External   External               `yaml:",omitempty" json:"external,omitempty"`
	Internal   bool                   `yaml:",omitempty" json:"internal,omitempty"`
	Attachable bool                   `yaml:",omitempty" json:"attachable,omitempty"`
	Labels     Labels                 `yaml:",omitempty" json:"labels,omitempty"`
	EnableIPv6 bool                   `mapstructure:"enable_ipv6" yaml:"enable_ipv6,omitempty" json:"enable_ipv6,omitempty"`
	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// IPAMConfig for a network
type IPAMConfig struct {
	Driver     string                 `yaml:",omitempty" json:"driver,omitempty"`
	Config     []*IPAMPool            `yaml:",omitempty" json:"config,omitempty"`
	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// IPAMPool for a network
type IPAMPool struct {
	Subnet             string                 `yaml:",omitempty" json:"subnet,omitempty"`
	Gateway            string                 `yaml:",omitempty" json:"gateway,omitempty"`
	IPRange            string                 `mapstructure:"ip_range" yaml:"ip_range,omitempty" json:"ip_range,omitempty"`
	AuxiliaryAddresses map[string]string      `mapstructure:"aux_addresses" yaml:"aux_addresses,omitempty" json:"aux_addresses,omitempty"`
	Extensions         map[string]interface{} `yaml:",inline" json:"-"`
}

// VolumeConfig for a volume
type VolumeConfig struct {
	Name       string                 `yaml:",omitempty" json:"name,omitempty"`
	Driver     string                 `yaml:",omitempty" json:"driver,omitempty"`
	DriverOpts map[string]string      `mapstructure:"driver_opts" yaml:"driver_opts,omitempty" json:"driver_opts,omitempty"`
	External   External               `yaml:",omitempty" json:"external,omitempty"`
	Labels     Labels                 `yaml:",omitempty" json:"labels,omitempty"`
	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// External identifies a Volume or Network as a reference to a resource that is
// not managed, and should already exist.
// External.name is deprecated and replaced by Volume.name
type External struct {
	Name       string                 `yaml:",omitempty" json:"name,omitempty"`
	External   bool                   `yaml:",omitempty" json:"external,omitempty"`
	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// MarshalYAML makes External implement yaml.Marshaller
func (e External) MarshalYAML() (interface{}, error) {
	if e.Name == "" {
		return e.External, nil
	}
	return External{Name: e.Name}, nil
}

// MarshalJSON makes External implement json.Marshaller
func (e External) MarshalJSON() ([]byte, error) {
	if e.Name == "" {
		return []byte(fmt.Sprintf("%v", e.External)), nil
	}
	return []byte(fmt.Sprintf(`{"name": %q}`, e.Name)), nil
}

// CredentialSpecConfig for credential spec on Windows
type CredentialSpecConfig struct {
	Config     string                 `yaml:",omitempty" json:"config,omitempty"` // Config was added in API v1.40
	File       string                 `yaml:",omitempty" json:"file,omitempty"`
	Registry   string                 `yaml:",omitempty" json:"registry,omitempty"`
	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

// FileObjectConfig is a config type for a file used by a service
type FileObjectConfig struct {
	Name           string                 `yaml:",omitempty" json:"name,omitempty"`
	File           string                 `yaml:",omitempty" json:"file,omitempty"`
	External       External               `yaml:",omitempty" json:"external,omitempty"`
	Labels         Labels                 `yaml:",omitempty" json:"labels,omitempty"`
	Driver         string                 `yaml:",omitempty" json:"driver,omitempty"`
	DriverOpts     map[string]string      `mapstructure:"driver_opts" yaml:"driver_opts,omitempty" json:"driver_opts,omitempty"`
	TemplateDriver string                 `mapstructure:"template_driver" yaml:"template_driver,omitempty" json:"template_driver,omitempty"`
	Extensions     map[string]interface{} `yaml:",inline" json:"-"`
}

const (
	// ServiceConditionCompletedSuccessfully is the type for waiting until a service has completed successfully (exit code 0).
	ServiceConditionCompletedSuccessfully = "service_completed_successfully"

	// ServiceConditionHealthy is the type for waiting until a service is healthy.
	ServiceConditionHealthy = "service_healthy"

	// ServiceConditionStarted is the type for waiting until a service has started (default).
	ServiceConditionStarted = "service_started"
)

type DependsOnConfig map[string]ServiceDependency

type ServiceDependency struct {
	Condition  string                 `yaml:",omitempty" json:"condition,omitempty"`
	Extensions map[string]interface{} `yaml:",inline" json:"-"`
}

type ExtendsConfig MappingWithEquals

// SecretConfig for a secret
type SecretConfig FileObjectConfig

// ConfigObjConfig is the config for the swarm "Config" object
type ConfigObjConfig FileObjectConfig
