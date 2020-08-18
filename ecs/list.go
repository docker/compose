/*
   Copyright 2020 Docker, Inc.

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

	"github.com/docker/api/compose"

	"github.com/compose-spec/compose-go/cli"
)

func (b *ecsAPIService) Ps(ctx context.Context, options *cli.ProjectOptions) ([]compose.ServiceStatus, error) {
	projectName, err := b.projectName(options)
	if err != nil {
		return nil, err
	}
	parameters, err := b.SDK.ListStackParameters(ctx, projectName)
	if err != nil {
		return nil, err
	}
	cluster := parameters[ParameterClusterName]

	resources, err := b.SDK.ListStackResources(ctx, projectName)
	if err != nil {
		return nil, err
	}

	servicesARN := []string{}
	for _, r := range resources {
		switch r.Type {
		case "AWS::ECS::Service":
			servicesARN = append(servicesARN, r.ARN)
		case "AWS::ECS::Cluster":
			cluster = r.ARN
		}
	}
	if len(servicesARN) == 0 {
		return nil, nil
	}
	status, err := b.SDK.DescribeServices(ctx, cluster, servicesARN)
	if err != nil {
		return nil, err
	}

	for i, state := range status {
		ports := []string{}
		for _, lb := range state.LoadBalancers {
			ports = append(ports, fmt.Sprintf(
				"%s:%d->%d/%s",
				lb.URL,
				lb.PublishedPort,
				lb.TargetPort,
				strings.ToLower(lb.Protocol)))
		}
		state.Ports = ports
		status[i] = state
	}
	return status, nil
}
