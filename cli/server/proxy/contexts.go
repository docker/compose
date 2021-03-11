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

package proxy

import (
	"context"

	"github.com/docker/compose-cli/api/config"
	"github.com/docker/compose-cli/api/context/store"
	contextsv1 "github.com/docker/compose-cli/cli/server/protos/contexts/v1"
)

type contextsProxy struct {
	configDir string
}

func (cp *contextsProxy) SetCurrent(ctx context.Context, request *contextsv1.SetCurrentRequest) (*contextsv1.SetCurrentResponse, error) {
	if err := config.WriteCurrentContext(cp.configDir, request.GetName()); err != nil {
		return &contextsv1.SetCurrentResponse{}, err
	}

	return &contextsv1.SetCurrentResponse{}, nil
}

func (cp *contextsProxy) List(ctx context.Context, request *contextsv1.ListRequest) (*contextsv1.ListResponse, error) {
	s := store.Instance()
	configFile, err := config.LoadFile(cp.configDir)
	if err != nil {
		return nil, err
	}
	contexts, err := s.List()
	if err != nil {
		return &contextsv1.ListResponse{}, err
	}

	return convertContexts(contexts, configFile.CurrentContext), nil
}

func convertContexts(contexts []*store.DockerContext, currentContext string) *contextsv1.ListResponse {
	result := &contextsv1.ListResponse{}

	for _, c := range contexts {
		endpointName := c.Type()
		if c.Type() == store.DefaultContextType {
			endpointName = "docker"
		}
		var endpoint interface{} = c.Endpoints[endpointName]

		context := contextsv1.Context{
			Name:        c.Name,
			ContextType: c.Type(),
			Description: c.Metadata.Description,
			Current:     c.Name == currentContext,
		}
		switch c.Type() {
		case store.DefaultContextType:
			context.Endpoint = getDockerEndpoint(endpoint)
		case store.AciContextType:
			context.Endpoint = getAciEndpoint(endpoint)
		case store.EcsContextType:
			context.Endpoint = getEcsEndpoint(endpoint)
		}

		result.Contexts = append(result.Contexts, &context)
	}
	return result
}

func getDockerEndpoint(endpoint interface{}) *contextsv1.Context_DockerEndpoint {
	typedEndpoint := endpoint.(*store.Endpoint)
	return &contextsv1.Context_DockerEndpoint{
		DockerEndpoint: &contextsv1.DockerEndpoint{
			Host: typedEndpoint.Host,
		},
	}
}

func getAciEndpoint(endpoint interface{}) *contextsv1.Context_AciEndpoint {
	typedEndpoint := endpoint.(*store.AciContext)
	return &contextsv1.Context_AciEndpoint{
		AciEndpoint: &contextsv1.AciEndpoint{
			ResourceGroup:  typedEndpoint.ResourceGroup,
			Region:         typedEndpoint.Location,
			SubscriptionId: typedEndpoint.SubscriptionID,
		},
	}
}

func getEcsEndpoint(endpoint interface{}) *contextsv1.Context_EcsEndpoint {
	typedEndpoint := endpoint.(*store.EcsContext)
	return &contextsv1.Context_EcsEndpoint{
		EcsEndpoint: &contextsv1.EcsEndpoint{
			FromEnvironment: typedEndpoint.CredentialsFromEnv,
			Profile:         typedEndpoint.Profile,
		},
	}
}
