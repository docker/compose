// +build ecs

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

	ecsplugin "github.com/docker/ecs-plugin/pkg/amazon/backend"

	"github.com/docker/api/backend"
	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/cloud"
	"github.com/docker/api/context/store"
	"github.com/docker/api/errdefs"
)

const backendType = store.EcsContextType

// ContextParams options for creating AWS context
type ContextParams struct {
	Description string
	Region      string
	Profile     string

	AwsID     string
	AwsSecret string
}

func init() {
	backend.Register(backendType, backendType, service, getCloudService)
}

func service(ctx context.Context) (backend.Service, error) {
	contextStore := store.ContextStore(ctx)
	currentContext := apicontext.CurrentContext(ctx)
	var ecsContext store.EcsContext

	if err := contextStore.GetEndpoint(currentContext, &ecsContext); err != nil {
		return nil, err
	}

	return getEcsAPIService(ecsContext)
}

func getEcsAPIService(ecsCtx store.EcsContext) (*ecsAPIService, error) {
	backend, err := ecsplugin.NewBackend(ecsCtx.Profile, ecsCtx.Region)
	if err != nil {
		return nil, err
	}
	return &ecsAPIService{
		ctx:            ecsCtx,
		composeBackend: backend,
	}, nil
}

type ecsAPIService struct {
	ctx            store.EcsContext
	composeBackend *ecsplugin.Backend
}

func (a *ecsAPIService) ContainerService() containers.Service {
	return nil
}

func (a *ecsAPIService) ComposeService() compose.Service {
	return a.composeBackend
}

func getCloudService() (cloud.Service, error) {
	return ecsCloudService{}, nil
}

type ecsCloudService struct {
}

func (a ecsCloudService) Login(ctx context.Context, params interface{}) error {
	return errdefs.ErrNotImplemented
}

func (a ecsCloudService) Logout(ctx context.Context) error {
	return errdefs.ErrNotImplemented
}

func (a ecsCloudService) CreateContextData(ctx context.Context, params interface{}) (interface{}, string, error) {
	contextHelper := newContextCreateHelper()
	createOpts := params.(ContextParams)
	return contextHelper.createContextData(ctx, createOpts)
}
