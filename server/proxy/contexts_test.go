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
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/compose-cli/context/store"
	contextsv1 "github.com/docker/compose-cli/protos/contexts/v1"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestConvertContext(t *testing.T) {
	contexts := []*store.DockerContext{
		{
			Name: store.DefaultContextName,
			Metadata: store.ContextMetadata{
				Description: "description 1",
				Type:        store.DefaultContextType,
			},
			Endpoints: map[string]interface{}{
				"docker": &store.Endpoint{
					Host: "unix://var/run/docker.sock",
				},
			},
		},
		{
			Name: "acicontext",
			Metadata: store.ContextMetadata{
				Description: "group1@eastus",
				Type:        store.AciContextType,
			},
			Endpoints: map[string]interface{}{
				"aci": &store.AciContext{
					Location:       "eastus",
					ResourceGroup:  "group1",
					SubscriptionID: "Subscription id",
				},
			},
		},
		{
			Name: "ecscontext",
			Metadata: store.ContextMetadata{
				Description: "ecs description",
				Type:        store.EcsContextType,
			},
			Endpoints: map[string]interface{}{
				"ecs": &store.EcsContext{
					CredentialsFromEnv: false,
					Profile:            "awsprofile",
				},
			},
		},
	}
	converted := convertContexts(contexts, "acicontext")
	expected := []*contextsv1.Context{
		{
			Name:        store.DefaultContextName,
			Current:     false,
			ContextType: store.DefaultContextType,
			Description: "description 1",
			Endpoint: &contextsv1.Context_DockerEndpoint{
				DockerEndpoint: &contextsv1.DockerEndpoint{
					Host: "unix://var/run/docker.sock",
				},
			},
		},
		{
			Name:        "acicontext",
			Current:     true,
			ContextType: store.AciContextType,
			Description: "group1@eastus",
			Endpoint: &contextsv1.Context_AciEndpoint{
				AciEndpoint: &contextsv1.AciEndpoint{
					Region:         "eastus",
					ResourceGroup:  "group1",
					SubscriptionId: "Subscription id",
				},
			},
		},
		{
			Name:        "ecscontext",
			Current:     false,
			ContextType: store.EcsContextType,
			Description: "ecs description",
			Endpoint: &contextsv1.Context_EcsEndpoint{
				EcsEndpoint: &contextsv1.EcsEndpoint{
					FromEnvironment: false,
					Profile:         "awsprofile",
				},
			},
		},
	}
	assert.DeepEqual(t, converted.Contexts, expected, cmpopts.IgnoreUnexported(contextsv1.Context{}, contextsv1.DockerEndpoint{}, contextsv1.AciEndpoint{}, contextsv1.EcsEndpoint{}))
}
