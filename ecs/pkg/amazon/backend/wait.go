package backend

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/docker/ecs-plugin/pkg/progress"
)

func (b *Backend) WaitStackCompletion(ctx context.Context, name string, operation int) error {
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

			resource := aws.StringValue(event.LogicalResourceId)
			reason := aws.StringValue(event.ResourceStatusReason)
			status := aws.StringValue(event.ResourceStatus)
			progressStatus := progress.Working

			switch status {
			case "CREATE_COMPLETE":
				if operation == compose.StackCreate {
					progressStatus = progress.Done

				}
			case "UPDATE_COMPLETE":
				if operation == compose.StackUpdate {
					progressStatus = progress.Done
				}
			case "DELETE_COMPLETE":
				if operation == compose.StackDelete {
					progressStatus = progress.Done
				}
			default:
				if strings.HasSuffix(status, "_FAILED") {
					progressStatus = progress.Error
					if stackErr == nil {
						operation = compose.StackDelete
						stackErr = fmt.Errorf(reason)
					}
				}
			}
			b.writer.Event(progress.Event{
				ID:         resource,
				Status:     progressStatus,
				StatusText: status,
			})
		}
	}

	return stackErr
}
