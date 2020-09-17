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
	"strings"
	"testing"

	"google.golang.org/grpc"
	"gotest.tools/v3/assert"

	containersv1 "github.com/docker/compose-cli/protos/containers/v1"
	contextsv1 "github.com/docker/compose-cli/protos/contexts/v1"
	streamsv1 "github.com/docker/compose-cli/protos/streams/v1"
	"github.com/docker/compose-cli/server/proxy"
)

func TestAllMethodsHaveCorrespondingCliCommand(t *testing.T) {
	s := setupServer()
	i := s.GetServiceInfo()
	for k, v := range i {
		if k == "grpc.health.v1.Health" {
			continue
		}
		var errs []string
		for _, m := range v.Methods {
			name := "/" + k + "/" + m.Name
			if _, keyExists := methodMapping[name]; !keyExists {
				errs = append(errs, name+" not mapped to a corresponding cli command")
			}
		}
		assert.Equal(t, "", strings.Join(errs, "\n"))
	}
}

func setupServer() *grpc.Server {
	ctx := context.TODO()
	s := New(ctx)
	p := proxy.New(ctx)
	containersv1.RegisterContainersServer(s, p)
	streamsv1.RegisterStreamingServer(s, p)
	contextsv1.RegisterContextsServer(s, p.ContextsProxy())
	return s
}
