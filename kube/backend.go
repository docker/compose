// +build kube

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

package kube

import (
	"context"

	"github.com/docker/compose-cli/api/backend"
	"github.com/docker/compose-cli/api/cloud"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	apicontext "github.com/docker/compose-cli/api/context"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/resources"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/api/volumes"
)

const backendType = store.KubeContextType

type kubeAPIService struct {
	ctx            store.KubeContext
	composeService compose.Service
}

func init() {
	backend.Register(backendType, backendType, service, cloud.NotImplementedCloudService)
}

func service(ctx context.Context) (backend.Service, error) {
	contextStore := store.ContextStore(ctx)
	currentContext := apicontext.CurrentContext(ctx)
	var kubeContext store.KubeContext

	if err := contextStore.GetEndpoint(currentContext, &kubeContext); err != nil {
		return nil, err
	}

	s, err := NewComposeService(kubeContext)
	if err != nil {
		return nil, err
	}
	return &kubeAPIService{
		ctx:            kubeContext,
		composeService: s,
	}, nil
}

func (s *kubeAPIService) ContainerService() containers.Service {
	return nil
}

func (s *kubeAPIService) ComposeService() compose.Service {
	return s.composeService
}

func (s *kubeAPIService) SecretsService() secrets.Service {
	return nil
}

func (s *kubeAPIService) VolumeService() volumes.Service {
	return nil
}

func (s *kubeAPIService) ResourceService() resources.Service {
	return nil
}
