package amazon

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	cf "github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/docker/ecs-plugin/pkg/console"
)

func (c *client) ComposeUp(ctx context.Context, project *compose.Project) error {
	ok, err := c.api.ClusterExists(ctx, c.Cluster)
	if err != nil {
		return err
	}
	if !ok {
		c.api.CreateCluster(ctx, c.Cluster)
	}
	update, err := c.api.StackExists(ctx, project.Name)
	if err != nil {
		return err
	}
	if update {
		return fmt.Errorf("we do not (yet) support updating an existing CloudFormation stack")
	}

	template, err := c.Convert(ctx, project)
	if err != nil {
		return err
	}

	err = c.api.CreateStack(ctx, project.Name, template)
	if err != nil {
		return err
	}

	w := console.NewProgressWriter()
	known := map[string]struct{}{}
	err = c.api.WaitStackComplete(ctx, project.Name, func() error {
		events, err := c.api.DescribeStackEvents(ctx, project.Name)
		if err != nil {
			return err
		}

		sort.Slice(events, func(i, j int) bool {
			return events[i].Timestamp.Before(*events[j].Timestamp)
		})

		for _, event := range events {
			if _, ok := known[*event.EventId]; ok {
				continue
			}
			known[*event.EventId] = struct{}{}

			resource := fmt.Sprintf("%s %q", aws.StringValue(event.ResourceType), aws.StringValue(event.LogicalResourceId))
			w.ResourceEvent(resource, aws.StringValue(event.ResourceStatus), aws.StringValue(event.ResourceStatusReason))
		}
		return nil
	})
	if err != nil {
		return err
	}

	// TODO monitor progress
	return nil
}

type upAPI interface {
	ClusterExists(ctx context.Context, name string) (bool, error)
	CreateCluster(ctx context.Context, name string) (string, error)
	StackExists(ctx context.Context, name string) (bool, error)
	CreateStack(ctx context.Context, name string, template *cloudformation.Template) error
	WaitStackComplete(ctx context.Context, name string, fn func() error) error
	DescribeStackEvents(ctx context.Context, stack string) ([]*cf.StackEvent, error)
}
