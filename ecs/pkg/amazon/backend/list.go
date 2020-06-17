package backend

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (b *Backend) Ps(ctx context.Context, project *types.Project) ([]compose.TaskStatus, error) {
	cluster := b.Cluster
	if cluster == "" {
		cluster = project.Name
	}
	arns := []string{}
	for _, service := range project.Services {
		tasks, err := b.api.ListTasks(ctx, cluster, service.Name)
		if err != nil {
			return []compose.TaskStatus{}, err
		}
		arns = append(arns, tasks...)
	}
	if len(arns) == 0 {
		return []compose.TaskStatus{}, nil
	}

	tasks, err := b.api.DescribeTasks(ctx, cluster, arns...)
	if err != nil {
		return []compose.TaskStatus{}, err
	}

	networkInterfaces := []string{}
	for _, t := range tasks {
		if t.NetworkInterface != "" {
			networkInterfaces = append(networkInterfaces, t.NetworkInterface)
		}
	}
	publicIps, err := b.api.GetPublicIPs(ctx, networkInterfaces...)
	if err != nil {
		return []compose.TaskStatus{}, err
	}

	sort.Slice(tasks, func(i, j int) bool {
		return strings.Compare(tasks[i].Service, tasks[j].Service) < 0
	})

	for i, task := range tasks {
		ports := []string{}
		s, err := project.GetService(task.Service)
		if err != nil {
			return []compose.TaskStatus{}, err
		}
		for _, p := range s.Ports {
			ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", publicIps[task.NetworkInterface], p.Published, p.Target, p.Protocol))
		}
		tasks[i].Name = s.Name
		tasks[i].Ports = ports
	}
	return tasks, nil
}
