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

	"github.com/docker/compose-cli/api/compose"

	"github.com/docker/compose-cli/progress"
)

func (b *ecsAPIService) Down(ctx context.Context, projectName string, options compose.DownOptions) error {
	resources, err := b.aws.ListStackResources(ctx, projectName)
	if err != nil {
		return err
	}

	err = resources.apply(awsTypeCapacityProvider, doDelete(ctx, b.aws.DeleteCapacityProvider))
	if err != nil {
		return err
	}

	err = resources.apply(awsTypeAutoscalingGroup, doDelete(ctx, b.aws.DeleteAutoscalingGroup))
	if err != nil {
		return err
	}

	previousEvents, err := b.previousStackEvents(ctx, projectName)
	if err != nil {
		return err
	}

	err = b.aws.DeleteStack(ctx, projectName)
	if err != nil {
		return err
	}
	return b.WaitStackCompletion(ctx, projectName, stackDelete, previousEvents...)
}

func (b *ecsAPIService) previousStackEvents(ctx context.Context, project string) ([]string, error) {
	events, err := b.aws.DescribeStackEvents(ctx, project)
	if err != nil {
		return nil, err
	}
	var previousEvents []string
	for _, e := range events {
		previousEvents = append(previousEvents, *e.EventId)
	}
	return previousEvents, nil
}

func doDelete(ctx context.Context, delete func(ctx context.Context, arn string) error) func(r stackResource) error {
	return func(r stackResource) error {
		w := progress.ContextWriter(ctx)
		w.Event(progress.RemovingEvent(r.LogicalID))
		err := delete(ctx, r.ARN)
		if err != nil {
			w.Event(progress.ErrorEvent(r.LogicalID))
			return err
		}
		w.Event(progress.RemovedEvent(r.LogicalID))
		return nil
	}
}
