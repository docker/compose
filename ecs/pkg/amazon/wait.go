package amazon

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/docker/ecs-plugin/pkg/console"
)

func (c *client) WaitStackCompletion(ctx context.Context, name string) error {
	w := console.NewProgressWriter()
	known := map[string]struct{}{}
	err := c.api.WaitStackComplete(ctx, name, func() error {
		events, err := c.api.DescribeStackEvents(ctx, name)
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
	return nil
}

type waitAPI interface {
	WaitStackComplete(ctx context.Context, name string, fn func() error) error
	DescribeStackEvents(ctx context.Context, stack string) ([]*cloudformation.StackEvent, error)
}
