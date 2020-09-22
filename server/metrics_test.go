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
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"gotest.tools/v3/assert"

	"github.com/docker/compose-cli/errdefs"
	"github.com/docker/compose-cli/metrics"
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

func TestTrackSuccess(t *testing.T) {
	var mockMetrics = &mockMetricsClient{}
	mockMetrics.On("Send", metrics.Command{Command: "ps", Context: "aci", Status: "success", Source: "api"}).Return()
	interceptor := metricsServerInterceptor(context.TODO(), mockMetrics)

	_, err := interceptor(incomingContext("aci"), nil, containerMethodRoute("List"), mockHandler(nil))
	assert.NilError(t, err)
}

func TestTrackSFailures(t *testing.T) {
	var mockMetrics = &mockMetricsClient{}
	mockMetrics.On("Send", metrics.Command{Command: "ps", Context: "default", Status: "failure", Source: "api"}).Return()
	interceptor := metricsServerInterceptor(context.TODO(), mockMetrics)

	_, err := interceptor(incomingContext("default"), nil, containerMethodRoute("Create"), mockHandler(errdefs.ErrLoginRequired))
	assert.Assert(t, err == errdefs.ErrLoginRequired)
}

func containerMethodRoute(action string) *grpc.UnaryServerInfo {
	var info = &grpc.UnaryServerInfo{
		FullMethod: "/com.docker.api.protos.containers.v1.Containers/" + action,
	}
	return info
}

func mockHandler(err error) func(ctx context.Context, req interface{}) (interface{}, error) {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, err
	}
}

func incomingContext(status string) context.Context {
	ctx := metadata.NewIncomingContext(context.TODO(), metadata.MD{
		(key): []string{status},
	})
	return ctx
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

type mockMetricsClient struct {
	mock.Mock
}

func (s *mockMetricsClient) Send(command metrics.Command) {
	s.Called(command)
}
