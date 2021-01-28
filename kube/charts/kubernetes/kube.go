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

package kubernetes

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	headlessPort      = 55555
	headlessName      = "headless"
	clusterIPHeadless = "None"
)

//MapToKubernetesObjects maps compose project to Kubernetes objects
func MapToKubernetesObjects(project *types.Project) (map[string]runtime.Object, error) {
	objects := map[string]runtime.Object{}

	for _, service := range project.Services {
		svcObject := mapToService(project, service)
		if svcObject != nil {
			objects[fmt.Sprintf("%s-service.yaml", service.Name)] = svcObject
		} else {
			log.Println("Missing port mapping from service config.")
		}

		if service.Deploy != nil && service.Deploy.Mode == "global" {
			daemonset, err := mapToDaemonset(project, service, project.Name)
			if err != nil {
				return nil, err
			}
			objects[fmt.Sprintf("%s-daemonset.yaml", service.Name)] = daemonset
		} else {
			deployment, err := mapToDeployment(project, service, project.Name)
			if err != nil {
				return nil, err
			}
			objects[fmt.Sprintf("%s-deployment.yaml", service.Name)] = deployment
		}
		for _, vol := range service.Volumes {
			if vol.Type == "volume" {
				vol.Source = strings.ReplaceAll(vol.Source, "_", "-")
				objects[fmt.Sprintf("%s-persistentvolumeclaim.yaml", vol.Source)] = mapToPVC(service, vol)
			}
		}
	}
	return objects, nil
}

func mapToService(project *types.Project, service types.ServiceConfig) *core.Service {
	ports := []core.ServicePort{}
	serviceType := core.ServiceTypeClusterIP
	clusterIP := ""
	for _, p := range service.Ports {
		if p.Published != 0 {
			serviceType = core.ServiceTypeLoadBalancer
		}
		ports = append(ports,
			core.ServicePort{
				Name:       fmt.Sprintf("%d-%s", p.Target, strings.ToLower(p.Protocol)),
				Port:       int32(p.Target),
				TargetPort: intstr.FromInt(int(p.Target)),
				Protocol:   toProtocol(p.Protocol),
			})
	}
	if len(ports) == 0 { // headless service
		clusterIP = clusterIPHeadless
		ports = append(ports, core.ServicePort{
			Name:       headlessName,
			Port:       headlessPort,
			TargetPort: intstr.FromInt(headlessPort),
			Protocol:   core.ProtocolTCP,
		})
	}
	return &core.Service{
		TypeMeta: meta.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: meta.ObjectMeta{
			Name: service.Name,
		},
		Spec: core.ServiceSpec{
			ClusterIP: clusterIP,
			Selector:  map[string]string{"com.docker.compose.service": service.Name},
			Ports:     ports,
			Type:      serviceType,
		},
	}
}

func mapToDeployment(project *types.Project, service types.ServiceConfig, name string) (*apps.Deployment, error) {
	labels := map[string]string{
		"com.docker.compose.service": service.Name,
		"com.docker.compose.project": name,
	}
	podTemplate, err := toPodTemplate(project, service, labels)
	if err != nil {
		return nil, err
	}
	selector := new(meta.LabelSelector)
	selector.MatchLabels = make(map[string]string)
	for key, val := range labels {
		selector.MatchLabels[key] = val
	}
	return &apps.Deployment{
		TypeMeta: meta.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: meta.ObjectMeta{
			Name:   service.Name,
			Labels: labels,
		},
		Spec: apps.DeploymentSpec{
			Selector: selector,
			Replicas: toReplicas(service.Deploy),
			Strategy: toDeploymentStrategy(service.Deploy),
			Template: podTemplate,
		},
	}, nil
}

func mapToDaemonset(project *types.Project, service types.ServiceConfig, name string) (*apps.DaemonSet, error) {
	labels := map[string]string{
		"com.docker.compose.service": service.Name,
		"com.docker.compose.project": name,
	}
	podTemplate, err := toPodTemplate(project, service, labels)
	if err != nil {
		return nil, err
	}

	return &apps.DaemonSet{
		ObjectMeta: meta.ObjectMeta{
			Name:   service.Name,
			Labels: labels,
		},
		Spec: apps.DaemonSetSpec{
			Template: podTemplate,
		},
	}, nil
}

func toReplicas(deploy *types.DeployConfig) *int32 {
	v := int32(1)
	if deploy != nil {
		v = int32(*deploy.Replicas)
	}
	return &v
}

func toDeploymentStrategy(deploy *types.DeployConfig) apps.DeploymentStrategy {
	if deploy == nil || deploy.UpdateConfig == nil {
		return apps.DeploymentStrategy{
			Type: apps.RecreateDeploymentStrategyType,
		}
	}
	return apps.DeploymentStrategy{
		Type: apps.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &apps.RollingUpdateDeployment{
			MaxUnavailable: &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: int32(*deploy.UpdateConfig.Parallelism),
			},
			MaxSurge: nil,
		},
	}
}

func mapToPVC(service types.ServiceConfig, vol types.ServiceVolumeConfig) runtime.Object {
	rwaccess := core.ReadWriteOnce
	if vol.ReadOnly {
		rwaccess = core.ReadOnlyMany
	}
	return &core.PersistentVolumeClaim{
		TypeMeta: meta.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
		},
		ObjectMeta: meta.ObjectMeta{
			Name:   vol.Source,
			Labels: map[string]string{"com.docker.compose.service": service.Name},
		},
		Spec: core.PersistentVolumeClaimSpec{
			VolumeName:  vol.Source,
			AccessModes: []core.PersistentVolumeAccessMode{rwaccess},
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
		},
	}
}

// toSecondsOrDefault converts a duration string in seconds and defaults to a
// given value if the duration is nil.
// The supported units are us, ms, s, m and h.
func toSecondsOrDefault(duration *types.Duration, defaultValue int32) int32 { //nolint: unparam
	if duration == nil {
		return defaultValue
	}
	return int32(time.Duration(*duration).Seconds())
}
