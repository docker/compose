package transform

import (
	"fmt"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/helm-prototype/pkg/compose"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	core "k8s.io/api/core/v1"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func MapToKubernetesObjects(model *compose.Project) (map[string]runtime.Object, error) {
	objects := map[string]runtime.Object{}
	for _, service :=  range model.Services {
		objects[fmt.Sprintf("%s-service.yaml", service.Name)] = mapToService(service)
		objects[fmt.Sprintf("%s-deployment.yaml", service.Name)] = mapToDeployment(service)
		for _, vol := range service.Volumes {
			if vol.Type == "volume" {
				objects[fmt.Sprintf("%s-persistentvolumeclain.yaml", service.Name)] = mapToPVC(service, vol)
			}
		}
	}
	return objects, nil
}

func mapToService(service types.ServiceConfig) *core.Service {
	return &core.Service{
		ObjectMeta: meta.ObjectMeta{
			Name:                       service.Name,
		},
		Spec:       core.ServiceSpec{
			Selector: map[string]string{"com.docker.compose.service": service.Name},
		},
	}
}

func mapToDeployment(service types.ServiceConfig) *apps.Deployment {
	return &apps.Deployment{
		ObjectMeta: meta.ObjectMeta{
			Name: service.Name,
			Labels: map[string]string{"com.docker.compose.service": service.Name},
		},
		Spec:       apps.DeploymentSpec{
				Template: core.PodTemplateSpec{
					ObjectMeta: meta.ObjectMeta{
						Labels: map[string]string{"com.docker.compose.service": service.Name},
					},
					Spec:       core.PodSpec{
						Containers: []core.Container{
							{
								Name: service.Name,
								Image: service.Image,
							},
						},
					},
				},
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
