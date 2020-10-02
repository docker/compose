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

	"github.com/docker/compose-cli/api/compose"
)

func (b *ecsAPIService) List(ctx context.Context, project string) ([]compose.Stack, error) {
	stacks, err := b.SDK.ListStacks(ctx, project)
	if err != nil {
		return nil, err
	}

	for _, stack := range stacks {
		if stack.Status == compose.STARTING {
			if err := b.checkStackState(ctx, stack.Name); err != nil {
				stack.Status = compose.FAILED
				stack.Reason = err.Error()
			}
		}
	}
	return stacks, nil

}

func (b *ecsAPIService) checkStackState(ctx context.Context, name string) error {
	resources, err := b.SDK.ListStackResources(ctx, name)
	if err != nil {
		return err
	}
	svcArns := []string{}
	svcNames := map[string]string{}
	var cluster string
	for _, r := range resources {
		if r.Type == "AWS::ECS::Cluster" {
			cluster = r.ARN
			continue
		}
		if r.Type == "AWS::ECS::Service" {
			if r.ARN == "" {
				continue
			}
			svcArns = append(svcArns, r.ARN)
			svcNames[r.ARN] = r.LogicalID
		}
	}
	if len(svcArns) == 0 {
		return nil
	}
	services, err := b.SDK.GetServiceTaskDefinition(ctx, cluster, svcArns)
	if err != nil {
		return err
	}
	for service, taskDef := range services {
		if err := b.checkServiceState(ctx, cluster, service, taskDef); err != nil {
			return fmt.Errorf("%s %s", svcNames[service], err.Error())
		}
	}
	return nil
}

func (b *ecsAPIService) checkServiceState(ctx context.Context, cluster string, service string, taskdef string) error {
	runningTasks, err := b.SDK.GetServiceTasks(ctx, cluster, service, false)
	if err != nil {
		return err
	}
	if len(runningTasks) > 0 {
		return nil
	}
	stoppedTasks, err := b.SDK.GetServiceTasks(ctx, cluster, service, true)
	if err != nil {
		return err
	}
	if len(stoppedTasks) == 0 {
		return nil
	}
	// filter tasks by task definition
	tasks := []string{}
	for _, t := range stoppedTasks {
		if t.TaskDefinitionArn != nil && *t.TaskDefinitionArn == taskdef {
			tasks = append(tasks, *t.TaskArn)
		}
	}
	if len(tasks) == 0 {
		return nil
	}
	reason, err := b.SDK.GetTaskStoppedReason(ctx, cluster, tasks[0])
	if err != nil {
		return err
	}
	return fmt.Errorf("%s", reason)
}
