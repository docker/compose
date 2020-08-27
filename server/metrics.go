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

package server

import (
	"context"

	"google.golang.org/grpc"

	"github.com/docker/compose-cli/metrics"
)

var (
	methodMapping = map[string]string{
		"/com.docker.api.protos.containers.v1.Containers/List":    "ps",
		"/com.docker.api.protos.containers.v1.Containers/Start":   "start",
		"/com.docker.api.protos.containers.v1.Containers/Stop":    "stop",
		"/com.docker.api.protos.containers.v1.Containers/Run":     "run",
		"/com.docker.api.protos.containers.v1.Containers/Exec":    "exec",
		"/com.docker.api.protos.containers.v1.Containers/Delete":  "rm",
		"/com.docker.api.protos.containers.v1.Containers/Kill":    "kill",
		"/com.docker.api.protos.containers.v1.Containers/Inspect": "inspect",
		"/com.docker.api.protos.containers.v1.Containers/Logs":    "logs",
		"/com.docker.api.protos.streams.v1.Streaming/NewStream":   "",
		"/com.docker.api.protos.context.v1.Contexts/List":         "context ls",
		"/com.docker.api.protos.context.v1.Contexts/SetCurrent":   "context use",
	}
)

func metricsServerInterceptor(clictx context.Context) grpc.UnaryServerInterceptor {
	client := metrics.NewClient()

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		currentContext, err := getIncomingContext(ctx)
		if err != nil {
			currentContext, err = getConfigContext(clictx)
			if err != nil {
				return nil, err
			}
		}

		command := methodMapping[info.FullMethod]
		if command != "" {
			client.Send(metrics.Command{
				Command: command,
				Context: currentContext,
				Source:  metrics.APISource,
			})
		}

		return handler(ctx, req)
	}
}
