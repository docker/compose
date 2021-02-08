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
)

func (b *ecsAPIService) Logs(ctx context.Context, projectName string, consumer compose.LogConsumer, options compose.LogOptions) error {
	if len(options.Services) > 0 {
		consumer = filteredLogConsumer(consumer, options.Services)
	}
	err := b.aws.GetLogs(ctx, projectName, consumer.Log, options.Follow)
	return err
}

func filteredLogConsumer(consumer compose.LogConsumer, services []string) compose.LogConsumer {
	if len(services) == 0 {
		return consumer
	}
	allowed := map[string]bool{}
	for _, s := range services {
		allowed[s] = true
	}
	return &allowListLogConsumer{
		allowList: allowed,
		delegate:  consumer,
	}
}

type allowListLogConsumer struct {
	allowList map[string]bool
	delegate  compose.LogConsumer
}

func (a *allowListLogConsumer) Log(service, container, message string) {
	if a.allowList[service] {
		a.delegate.Log(service, container, message)
	}
}

func (a *allowListLogConsumer) Status(service, container, message string) {
	if a.allowList[service] {
		a.delegate.Status(service, container, message)
	}
}
