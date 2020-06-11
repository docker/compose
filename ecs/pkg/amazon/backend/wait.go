package backend

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/ecs-plugin/pkg/console"
)

func (b *Backend) WaitStackCompletion(ctx context.Context, name string, operation int) error {
	w := console.NewProgressWriter()
	knownEvents := map[string]struct{}{}

	// Get the unique Stack ID so we can collect events without getting some from previous deployments with same name
	stackID, err := b.api.GetStackID(ctx, name)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(1 * time.Second)
	done := make(chan bool)

	go func() {
		b.api.WaitStackComplete(ctx, stackID, operation) //nolint:errcheck
		ticker.Stop()
		done <- true
	}()

	var completed bool
	var stackErr error
	for !completed {
		select {
		case <-done:
			completed = true
		case <-ticker.C:
		}
		events, err := b.api.DescribeStackEvents(ctx, stackID)
		if err != nil {
			return err
		}

		sort.Slice(events, func(i, j int) bool {
			return events[i].Timestamp.Before(*events[j].Timestamp)
		})

		for _, event := range events {
			if _, ok := knownEvents[*event.EventId]; ok {
				continue
			}
			knownEvents[*event.EventId] = struct{}{}

			resource := fmt.Sprintf("%s %q", aws.StringValue(event.ResourceType), aws.StringValue(event.LogicalResourceId))
			reason := aws.StringValue(event.ResourceStatusReason)
			status := aws.StringValue(event.ResourceStatus)
			w.ResourceEvent(resource, status, reason)
			if stackErr == nil && strings.HasSuffix(status, "_FAILED") {
				stackErr = fmt.Errorf(reason)
			}
		}
	}
	return stackErr
}
