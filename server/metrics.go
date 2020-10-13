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

package server

import (
	"context"

	"google.golang.org/grpc"

	"github.com/docker/compose-cli/metrics"
	"github.com/docker/compose-cli/server/proxy"
)

var (
	methodMapping = map[string]string{
		"/com.docker.api.protos.containers.v1.Containers/List":     "ps",
		"/com.docker.api.protos.containers.v1.Containers/Start":    "start",
		"/com.docker.api.protos.containers.v1.Containers/Stop":     "stop",
		"/com.docker.api.protos.containers.v1.Containers/Run":      "run",
		"/com.docker.api.protos.containers.v1.Containers/Exec":     "exec",
		"/com.docker.api.protos.containers.v1.Containers/Delete":   "rm",
		"/com.docker.api.protos.containers.v1.Containers/Kill":     "kill",
		"/com.docker.api.protos.containers.v1.Containers/Inspect":  "inspect",
		"/com.docker.api.protos.containers.v1.Containers/Logs":     "logs",
		"/com.docker.api.protos.streams.v1.Streaming/NewStream":    "streaming",
		"/com.docker.api.protos.context.v1.Contexts/List":          "context ls",
		"/com.docker.api.protos.context.v1.Contexts/SetCurrent":    "context use",
		"/com.docker.api.protos.volumes.v1.Volumes/VolumesList":    "volume ls",
		"/com.docker.api.protos.volumes.v1.Volumes/VolumesDelete":  "volume rm",
		"/com.docker.api.protos.volumes.v1.Volumes/VolumesCreate":  "volume create",
		"/com.docker.api.protos.volumes.v1.Volumes/VolumesInspect": "volume inspect",
	}
)

func metricsServerInterceptor(client metrics.Client) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		backendClient := proxy.Client(ctx)
		contextType := ""
		if backendClient != nil {
			contextType = backendClient.ContextType()
		}

		data, err := handler(ctx, req)

		status := metrics.SuccessStatus
		if err != nil {
			status = metrics.FailureStatus
		}
		command := methodMapping[info.FullMethod]
		if command != "" {
			client.Send(metrics.Command{
				Command: command,
				Context: contextType,
				Source:  metrics.APISource,
				Status:  status,
			})
		}
		return data, err
	}
}
