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

package ecs

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/docker/compose-cli/progress"

	"github.com/aws/aws-sdk-go/aws"
)

func (b *ecsAPIService) WaitStackCompletion(ctx context.Context, name string, operation int, ignored ...string) error { //nolint:gocyclo
	knownEvents := map[string]struct{}{}
	for _, id := range ignored {
		knownEvents[id] = struct{}{}
	}

	// progress writer
	w := progress.ContextWriter(ctx)
	// Get the unique Stack ID so we can collect events without getting some from previous deployments with same name
	stackID, err := b.SDK.GetStackID(ctx, name)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(1 * time.Second)
	done := make(chan bool)
	go func() {
		b.SDK.WaitStackComplete(ctx, stackID, operation) //nolint:errcheck
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
		events, err := b.SDK.DescribeStackEvents(ctx, stackID)
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
				if operation == stackCreate {
					progressStatus = progress.Done
				}
			case "UPDATE_COMPLETE":
				if operation == stackUpdate {
					progressStatus = progress.Done
				}
			case "DELETE_COMPLETE":
				if operation == stackDelete {
					progressStatus = progress.Done
				}
			default:
				if strings.HasSuffix(status, "_FAILED") {
					progressStatus = progress.Error
					if stackErr == nil {
						operation = stackDelete
						stackErr = fmt.Errorf(reason)
					}
				}
			}
			w.Event(progress.Event{
				ID:         resource,
				Status:     progressStatus,
				StatusText: status,
			})
		}
	}

	return stackErr
}
