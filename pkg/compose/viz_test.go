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

package compose

import (
	"context"
	"strconv"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	compose "github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/mocks"
)

func TestViz(t *testing.T) {
	project := types.Project{
		Name:       "viz-test",
		WorkingDir: "/home",
		Services: types.Services{
			"service1": {
				Name:  "service1",
				Image: "image-for-service1",
				Ports: []types.ServicePortConfig{
					{
						Published: "80",
						Target:    80,
						Protocol:  "tcp",
					},
					{
						Published: "53",
						Target:    533,
						Protocol:  "udp",
					},
				},
				Networks: map[string]*types.ServiceNetworkConfig{
					"internal": nil,
				},
			},
			"service2": {
				Name:  "service2",
				Image: "image-for-service2",
				Ports: []types.ServicePortConfig{},
			},
			"service3": {
				Name:  "service3",
				Image: "some-image",
				DependsOn: map[string]types.ServiceDependency{
					"service2": {},
					"service1": {},
				},
			},
			"service4": {
				Name:  "service4",
				Image: "another-image",
				DependsOn: map[string]types.ServiceDependency{
					"service3": {},
				},
				Ports: []types.ServicePortConfig{
					{
						Published: "8080",
						Target:    80,
					},
				},
				Networks: map[string]*types.ServiceNetworkConfig{
					"external": nil,
				},
			},
			"With host IP": {
				Name:  "With host IP",
				Image: "user/image-name",
				DependsOn: map[string]types.ServiceDependency{
					"service1": {},
				},
				Ports: []types.ServicePortConfig{
					{
						Published: "8888",
						Target:    8080,
						HostIP:    "127.0.0.1",
					},
				},
			},
		},
		Networks: types.Networks{
			"internal": types.NetworkConfig{},
			"external": types.NetworkConfig{},
			"not-used": types.NetworkConfig{},
		},
		Volumes:          nil,
		Secrets:          nil,
		Configs:          nil,
		Extensions:       nil,
		ComposeFiles:     nil,
		Environment:      nil,
		DisabledServices: nil,
		Profiles:         nil,
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	cli := mocks.NewMockCli(mockCtrl)
	tested := composeService{
		dockerCli: cli,
	}

	ctx := context.Background()

	t.Run("viz (no ports, networks or image)", func(t *testing.T) {
		graphStr, err := tested.Viz(ctx, &project, compose.VizOptions{
			Indentation:      "  ",
			IncludePorts:     false,
			IncludeImageName: false,
			IncludeNetworks:  false,
		})
		require.NoError(t, err, "viz command failed")

		// check indentation
		assert.Contains(t, graphStr, "\n  ", graphStr)
		assert.NotContains(t, graphStr, "\n   ", graphStr)

		// check digraph name
		assert.Contains(t, graphStr, "digraph \""+project.Name+"\"", graphStr)

		// check nodes
		for _, service := range project.Services {
			assert.Contains(t, graphStr, "\""+service.Name+"\" [style=\"filled\"", graphStr)
		}

		// check node attributes
		assert.NotContains(t, graphStr, "Networks", graphStr)
		assert.NotContains(t, graphStr, "Image", graphStr)
		assert.NotContains(t, graphStr, "Ports", graphStr)

		// check edges that SHOULD exist in the generated graph
		allowedEdges := make(map[string][]string)
		for name, service := range project.Services {
			allowed := make([]string, 0, len(service.DependsOn))
			for depName := range service.DependsOn {
				allowed = append(allowed, depName)
			}
			allowedEdges[name] = allowed
		}
		for serviceName, dependencies := range allowedEdges {
			for _, dependencyName := range dependencies {
				assert.Contains(t, graphStr, "\""+serviceName+"\" -> \""+dependencyName+"\"", graphStr)
			}
		}

		// check edges that SHOULD NOT exist in the generated graph
		forbiddenEdges := make(map[string][]string)
		for name, service := range project.Services {
			forbiddenEdges[name] = make([]string, 0, len(project.ServiceNames())-len(service.DependsOn))
			for _, serviceName := range project.ServiceNames() {
				_, edgeExists := service.DependsOn[serviceName]
				if !edgeExists {
					forbiddenEdges[name] = append(forbiddenEdges[name], serviceName)
				}
			}
		}
		for serviceName, forbiddenDeps := range forbiddenEdges {
			for _, forbiddenDep := range forbiddenDeps {
				assert.NotContains(t, graphStr, "\""+serviceName+"\" -> \""+forbiddenDep+"\"")
			}
		}
	})

	t.Run("viz (with ports, networks and image)", func(t *testing.T) {
		graphStr, err := tested.Viz(ctx, &project, compose.VizOptions{
			Indentation:      "\t",
			IncludePorts:     true,
			IncludeImageName: true,
			IncludeNetworks:  true,
		})
		require.NoError(t, err, "viz command failed")

		// check indentation
		assert.Contains(t, graphStr, "\n\t", graphStr)
		assert.NotContains(t, graphStr, "\n\t\t", graphStr)

		// check digraph name
		assert.Contains(t, graphStr, "digraph \""+project.Name+"\"", graphStr)

		// check nodes
		for _, service := range project.Services {
			assert.Contains(t, graphStr, "\""+service.Name+"\" [style=\"filled\"", graphStr)
		}

		// check node attributes
		assert.Contains(t, graphStr, "Networks", graphStr)
		assert.Contains(t, graphStr, ">internal<", graphStr)
		assert.Contains(t, graphStr, ">external<", graphStr)
		assert.Contains(t, graphStr, "Image", graphStr)
		for _, service := range project.Services {
			assert.Contains(t, graphStr, ">"+service.Image+"<", graphStr)
		}
		assert.Contains(t, graphStr, "Ports", graphStr)
		for _, service := range project.Services {
			for _, portConfig := range service.Ports {
				assert.NotContains(t, graphStr, ">"+portConfig.Published+":"+strconv.Itoa(int(portConfig.Target))+"<", graphStr)
			}
		}
	})
}
