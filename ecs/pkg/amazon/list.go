package amazon

import (
	"context"
	"fmt"
	"os"
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
	for _, s := range project.Services {
		tasks, err := c.api.GetTasks(ctx, cluster, s.Name)
		if err != nil {
			return err
		}
		if len(tasks) == 0 {
			continue
		}
		// TODO get more data from DescribeTask, including tasks status
		networkInterfaces, err := c.api.GetNetworkInterfaces(ctx, cluster, tasks...)
		if err != nil {
			return err
		}
		if len(networkInterfaces) == 0 {
			fmt.Fprintf(w, "%s\t%s\t\n", s.Name, "Provisioning")
			continue
		}
		publicIps, err := c.api.GetPublicIPs(ctx, networkInterfaces...)
		if err != nil {
			return err
		}
		ports := []string{}
		for _, p := range s.Ports {
			ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", strings.Join(publicIps, ","), p.Published, p.Target, p.Protocol))
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, "Up", strings.Join(ports, ", "))
	}
	w.Flush()
	return nil
}

type psAPI interface {
	GetTasks(ctx context.Context, cluster string, name string) ([]string, error)
	GetNetworkInterfaces(ctx context.Context, cluster string, arns ...string) ([]string, error)
	GetPublicIPs(ctx context.Context, interfaces ...string) ([]string, error)
}
