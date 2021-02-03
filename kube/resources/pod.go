// +build kube

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

package resources

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/docker/api/types/swarm"

	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func toPodTemplate(project *types.Project, serviceConfig types.ServiceConfig, labels map[string]string) (apiv1.PodTemplateSpec, error) {
	tpl := apiv1.PodTemplateSpec{}
	//nodeAffinity, err := toNodeAffinity(serviceConfig.Deploy)
	//if err != nil {
	//	return apiv1.PodTemplateSpec{}, err
	//}
	hostAliases, err := toHostAliases(serviceConfig.ExtraHosts)
	if err != nil {
		return apiv1.PodTemplateSpec{}, err
	}
	env, err := toEnv(serviceConfig.Environment)
	if err != nil {
		return apiv1.PodTemplateSpec{}, err
	}
	restartPolicy, err := toRestartPolicy(serviceConfig)
	if err != nil {
		return apiv1.PodTemplateSpec{}, err
	}

	var limits apiv1.ResourceList
	if serviceConfig.Deploy != nil && serviceConfig.Deploy.Resources.Limits != nil {
		limits, err = toResource(serviceConfig.Deploy.Resources.Limits)
		if err != nil {
			return apiv1.PodTemplateSpec{}, err
		}
	}
	var requests apiv1.ResourceList
	if serviceConfig.Deploy != nil && serviceConfig.Deploy.Resources.Reservations != nil {
		requests, err = toResource(serviceConfig.Deploy.Resources.Reservations)
		if err != nil {
			return apiv1.PodTemplateSpec{}, err
		}
	}

	volumes, err := toVolumes(project, serviceConfig)
	if err != nil {
		return apiv1.PodTemplateSpec{}, err
	}
	volumeMounts, err := toVolumeMounts(project, serviceConfig)
	if err != nil {
		return apiv1.PodTemplateSpec{}, err
	}
	/*	pullPolicy, err := toImagePullPolicy(serviceConfig.Image, x-kubernetes-pull-policy)
		if err != nil {
			return apiv1.PodTemplateSpec{}, err
		} */
	tpl.ObjectMeta = metav1.ObjectMeta{
		Labels:      labels,
		Annotations: serviceConfig.Labels,
	}
	tpl.Spec.RestartPolicy = restartPolicy
	tpl.Spec.Volumes = volumes
	tpl.Spec.HostPID = toHostPID(serviceConfig.Pid)
	tpl.Spec.HostIPC = toHostIPC(serviceConfig.Ipc)
	tpl.Spec.Hostname = serviceConfig.Hostname
	tpl.Spec.TerminationGracePeriodSeconds = toTerminationGracePeriodSeconds(serviceConfig.StopGracePeriod)
	tpl.Spec.HostAliases = hostAliases
	//tpl.Spec.Affinity = nodeAffinity
	// we dont want to remove all containers and recreate them because:
	// an admission plugin can add sidecar containers
	// we for sure want to keep the main container to be additive
	if len(tpl.Spec.Containers) == 0 {
		tpl.Spec.Containers = []apiv1.Container{{}}
	}

	containerIX := 0
	for ix, c := range tpl.Spec.Containers {
		if c.Name == serviceConfig.Name {
			containerIX = ix
			break
		}
	}
	tpl.Spec.Containers[containerIX].Name = serviceConfig.Name
	tpl.Spec.Containers[containerIX].Image = serviceConfig.Image
	// FIXME tpl.Spec.Containers[containerIX].ImagePullPolicy = pullPolicy
	tpl.Spec.Containers[containerIX].Command = serviceConfig.Entrypoint
	tpl.Spec.Containers[containerIX].Args = serviceConfig.Command
	tpl.Spec.Containers[containerIX].WorkingDir = serviceConfig.WorkingDir
	tpl.Spec.Containers[containerIX].TTY = serviceConfig.Tty
	tpl.Spec.Containers[containerIX].Stdin = serviceConfig.StdinOpen
	tpl.Spec.Containers[containerIX].Ports = toPorts(serviceConfig.Ports)
	tpl.Spec.Containers[containerIX].LivenessProbe = toLivenessProbe(serviceConfig.HealthCheck)
	tpl.Spec.Containers[containerIX].Env = env
	tpl.Spec.Containers[containerIX].VolumeMounts = volumeMounts
	tpl.Spec.Containers[containerIX].SecurityContext = toSecurityContext(serviceConfig)
	tpl.Spec.Containers[containerIX].Resources = apiv1.ResourceRequirements{
		Limits:   limits,
		Requests: requests,
	}

	/* FIXME
	if serviceConfig.PullSecret != "" {
		pullSecrets := map[string]struct{}{}
		for _, ps := range tpl.Spec.ImagePullSecrets {
			pullSecrets[ps.Name] = struct{}{}
		}
		if _, ok := pullSecrets[serviceConfig.PullSecret]; !ok {
			tpl.Spec.ImagePullSecrets = append(tpl.Spec.ImagePullSecrets, apiv1.LocalObjectReference{Name: serviceConfig.PullSecret})
		}
	}
	*/
	return tpl, nil
}

func toHostAliases(extraHosts []string) ([]apiv1.HostAlias, error) {
	if extraHosts == nil {
		return nil, nil
	}

	byHostnames := map[string]string{}
	for _, host := range extraHosts {
		split := strings.SplitN(host, ":", 2)
		if len(split) != 2 {
			return nil, errors.Errorf("malformed host %s", host)
		}
		byHostnames[split[0]] = split[1]
	}

	byIPs := map[string][]string{}
	for k, v := range byHostnames {
		byIPs[v] = append(byIPs[v], k)
	}

	aliases := make([]apiv1.HostAlias, len(byIPs))
	i := 0
	for key, hosts := range byIPs {
		sort.Strings(hosts)
		aliases[i] = apiv1.HostAlias{
			IP:        key,
			Hostnames: hosts,
		}
		i++
	}
	sort.Slice(aliases, func(i, j int) bool { return aliases[i].IP < aliases[j].IP })
	return aliases, nil
}

func toHostPID(pid string) bool {
	return "host" == pid
}

func toHostIPC(ipc string) bool {
	return "host" == ipc
}

func toTerminationGracePeriodSeconds(duration *types.Duration) *int64 {
	if duration == nil {
		return nil
	}
	gracePeriod := int64(time.Duration(*duration).Seconds())
	return &gracePeriod
}

func toLivenessProbe(hc *types.HealthCheckConfig) *apiv1.Probe {
	if hc == nil || len(hc.Test) < 1 || hc.Test[0] == "NONE" {
		return nil
	}

	command := hc.Test[1:]
	if hc.Test[0] == "CMD-SHELL" {
		command = append([]string{"sh", "-c"}, command...)
	}

	return &apiv1.Probe{
		TimeoutSeconds:   toSecondsOrDefault(hc.Timeout, 1),
		PeriodSeconds:    toSecondsOrDefault(hc.Interval, 1),
		FailureThreshold: int32(defaultUint64(hc.Retries, 3)),
		Handler: apiv1.Handler{
			Exec: &apiv1.ExecAction{
				Command: command,
			},
		},
	}
}

func toEnv(env map[string]*string) ([]apiv1.EnvVar, error) {
	var envVars []apiv1.EnvVar

	for k, v := range env {
		if v == nil {
			return nil, errors.Errorf("%s has no value, unsetting an environment variable is not supported", k)
		}
		envVars = append(envVars, toEnvVar(k, *v))
	}
	sort.Slice(envVars, func(i, j int) bool { return envVars[i].Name < envVars[j].Name })
	return envVars, nil
}

func toEnvVar(key, value string) apiv1.EnvVar {
	return apiv1.EnvVar{
		Name:  key,
		Value: value,
	}
}

func toPorts(list []types.ServicePortConfig) []apiv1.ContainerPort {
	var ports []apiv1.ContainerPort

	for _, v := range list {
		ports = append(ports, apiv1.ContainerPort{
			ContainerPort: int32(v.Target),
			Protocol:      toProtocol(v.Protocol),
		})
	}

	return ports
}

func toProtocol(value string) apiv1.Protocol {
	if value == "udp" {
		return apiv1.ProtocolUDP
	}
	return apiv1.ProtocolTCP
}

func toRestartPolicy(s types.ServiceConfig) (apiv1.RestartPolicy, error) {
	if s.Deploy == nil || s.Deploy.RestartPolicy == nil {
		return apiv1.RestartPolicyAlways, nil
	}
	policy := s.Deploy.RestartPolicy

	switch policy.Condition {
	case string(swarm.RestartPolicyConditionAny):
		return apiv1.RestartPolicyAlways, nil
	case string(swarm.RestartPolicyConditionNone):
		return apiv1.RestartPolicyNever, nil
	case string(swarm.RestartPolicyConditionOnFailure):
		return apiv1.RestartPolicyOnFailure, nil
	default:
		return "", errors.Errorf("unsupported restart policy %s", policy.Condition)
	}
}

func toResource(res *types.Resource) (apiv1.ResourceList, error) {
	list := make(apiv1.ResourceList)
	if res.NanoCPUs != "" {
		cpus, err := resource.ParseQuantity(res.NanoCPUs)
		if err != nil {
			return nil, err
		}
		list[apiv1.ResourceCPU] = cpus
	}
	if res.MemoryBytes != 0 {
		memory, err := resource.ParseQuantity(fmt.Sprintf("%v", res.MemoryBytes))
		if err != nil {
			return nil, err
		}
		list[apiv1.ResourceMemory] = memory
	}
	return list, nil
}

func toSecurityContext(s types.ServiceConfig) *apiv1.SecurityContext {
	isPrivileged := toBoolPointer(s.Privileged)
	isReadOnly := toBoolPointer(s.ReadOnly)

	var capabilities *apiv1.Capabilities
	if s.CapAdd != nil || s.CapDrop != nil {
		capabilities = &apiv1.Capabilities{
			Add:  toCapabilities(s.CapAdd),
			Drop: toCapabilities(s.CapDrop),
		}
	}

	var userID *int64
	if s.User != "" {
		numerical, err := strconv.Atoi(s.User)
		if err == nil {
			unixUserID := int64(numerical)
			userID = &unixUserID
		}
	}

	if isPrivileged == nil && isReadOnly == nil && capabilities == nil && userID == nil {
		return nil
	}

	return &apiv1.SecurityContext{
		RunAsUser:              userID,
		Privileged:             isPrivileged,
		ReadOnlyRootFilesystem: isReadOnly,
		Capabilities:           capabilities,
	}
}

func toBoolPointer(value bool) *bool {
	if value {
		return &value
	}

	return nil
}

func defaultUint64(v *uint64, defaultValue uint64) uint64 { //nolint: unparam
	if v == nil {
		return defaultValue
	}

	return *v
}

func toCapabilities(list []string) (capabilities []apiv1.Capability) {
	for _, c := range list {
		capabilities = append(capabilities, apiv1.Capability(c))
	}
	return
}
