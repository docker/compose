package convert

import (
	"fmt"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/helm-prototype/pkg/compose"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strings"
	"time"
)

func MapToKubernetesObjects(model *compose.Project) (map[string]runtime.Object, error) {
	objects := map[string]runtime.Object{}
	for _, service :=  range model.Services {
		objects[fmt.Sprintf("%s-service.yaml", service.Name)] = mapToService(model, service)
		if service.Deploy != nil && service.Deploy.Mode == "global" {
			daemonset, err := mapToDaemonset(service, model)
			if err != nil {
				return nil, err
			}
			objects[fmt.Sprintf("%s-daemonset.yaml", service.Name)] = daemonset
		} else {
			deployment, err := mapToDeployment(service, model)
			if err != nil {
				return nil, err
			}
			objects[fmt.Sprintf("%s-deployment.yaml", service.Name)] = deployment
		}
		for _, vol := range service.Volumes {
			if vol.Type == "volume" {
				objects[fmt.Sprintf("%s-persistentvolumeclain.yaml", service.Name)] = mapToPVC(service, vol)
			}
		}
	}
	return objects, nil
}

func mapToService(model *compose.Project, service types.ServiceConfig) *core.Service {
	ports := []core.ServicePort{}
	for _, p := range service.Ports {
		ports = append(ports,
			core.ServicePort{
				Name:       fmt.Sprintf("%d-%s", p.Target, strings.ToLower(string(p.Protocol))),
				Port:       int32(p.Target),
				TargetPort: intstr.FromInt(int(p.Target)),
				Protocol:   toProtocol(p.Protocol),
			})
	}

	return &core.Service{
		ObjectMeta: meta.ObjectMeta{
			Name: service.Name,
		},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"com.docker.compose.service": service.Name},
			Ports:    ports,
			Type:     mapServiceToServiceType(service, model),
		},
	}
}

func mapServiceToServiceType(service types.ServiceConfig, model *compose.Project) core.ServiceType {
	serviceType := core.ServiceTypeClusterIP
	if len(service.Networks) == 0 {
		// service is implicitly attached to "default" network
		serviceType = core.ServiceTypeLoadBalancer
	}
	for name := range service.Networks {
		if !model.Networks[name].Internal {
			serviceType = core.ServiceTypeLoadBalancer
		}
	}
	for _, port := range service.Ports {
		if port.Published != 0 {
			serviceType = core.ServiceTypeNodePort
		}
	}
	return serviceType
}

func mapToDeployment(service types.ServiceConfig, model *compose.Project) (*apps.Deployment, error) {
	labels := map[string]string{
		"com.docker.compose.service": service.Name,
		"com.docker.compose.project": model.Name,
	}
	podTemplate, err := toPodTemplate(service, labels, model)
	if err != nil {
		return nil, err
	}

	return &apps.Deployment{
		ObjectMeta: meta.ObjectMeta{
			Name: service.Name,
			Labels: labels,
		},
		Spec:       apps.DeploymentSpec{
				Replicas: toReplicas(service.Deploy),
				Strategy: toDeploymentStrategy(service.Deploy),
				Template: podTemplate,
		},
	}, nil
}

func mapToDaemonset(service types.ServiceConfig, model *compose.Project) (*apps.DaemonSet, error) {
	labels := map[string]string{
		"com.docker.compose.service": service.Name,
		"com.docker.compose.project": model.Name,
	}
	podTemplate, err := toPodTemplate(service, labels, model)
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
	return &core.PersistentVolumeClaim{
		ObjectMeta: meta.ObjectMeta{
			Name:                       vol.Source,
			Labels:                     map[string]string{"com.docker.compose.service": service.Name},
		},
		Spec:       core.PersistentVolumeClaimSpec{
			VolumeName:       vol.Source,
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
