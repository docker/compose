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
	"strings"

	"github.com/docker/compose-cli/api/compose"
)

func (b *ecsAPIService) Ps(ctx context.Context, project string) ([]compose.ServiceStatus, error) {
	cluster, err := b.aws.GetStackClusterID(ctx, project)
	if err != nil {
		return nil, err
	}
	servicesARN, err := b.aws.ListStackServices(ctx, project)
	if err != nil {
		return nil, err
	}

	if len(servicesARN) == 0 {
		return nil, nil
	}

	status := []compose.ServiceStatus{}
	for _, arn := range servicesARN {
		state, err := b.aws.DescribeService(ctx, cluster, arn)
		if err != nil {
			return nil, err
		}
		ports := []string{}
		for _, lb := range state.Publishers {
			ports = append(ports, fmt.Sprintf(
				"%s:%d->%d/%s",
				lb.URL,
				lb.PublishedPort,
				lb.TargetPort,
				strings.ToLower(lb.Protocol)))
		}
		state.Ports = ports
		status = append(status, state)
	}
	return status, nil
}
