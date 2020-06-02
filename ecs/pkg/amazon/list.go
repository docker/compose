package amazon

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/docker/ecs-plugin/pkg/compose"
)

func (c *client) ComposePs(ctx context.Context, project *compose.Project) error {
	cluster := c.Cluster
	if cluster == "" {
		cluster = project.Name
	}
	w := tabwriter.NewWriter(os.Stdout, 20, 2, 3, ' ', 0)
	fmt.Fprintf(w, "Name\tState\tPorts\n")
	defer w.Flush()

	arns := []string{}
	for _, service := range project.Services {
		tasks, err := c.api.ListTasks(ctx, cluster, service.Name)
		if err != nil {
			return err
		}
		arns = append(arns, tasks...)
	}
	if len(arns) == 0 {
		return nil
	}

	tasks, err := c.api.DescribeTasks(ctx, cluster, arns...)
	if err != nil {
		return err
	}

	networkInterfaces := []string{}
	for _, t := range tasks {
		if t.NetworkInterface != "" {
			networkInterfaces = append(networkInterfaces, t.NetworkInterface)
		}
	}
	publicIps, err := c.api.GetPublicIPs(ctx, networkInterfaces...)
	if err != nil {
		return err
	}

	sort.Slice(tasks, func(i, j int) bool {
		return strings.Compare(tasks[i].Service, tasks[j].Service) < 0
	})

	for _, t := range tasks {
		ports := []string{}
		s, err := project.GetService(t.Service)
		if err != nil {
			return err
		}
		for _, p := range s.Ports {
			ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", publicIps[t.NetworkInterface], p.Published, p.Target, p.Protocol))
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, t.State, strings.Join(ports, ", "))
	}
	return nil
}

type TaskStatus struct {
	State            string
	Service          string
	NetworkInterface string
	PublicIP         string
}

type listAPI interface {
	ListTasks(ctx context.Context, cluster string, name string) ([]string, error)
	DescribeTasks(ctx context.Context, cluster string, arns ...string) ([]TaskStatus, error)
	GetPublicIPs(ctx context.Context, interfaces ...string) (map[string]string, error)
}
