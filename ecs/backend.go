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
	"fmt"

	"github.com/docker/compose-cli/api/backend"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/api/resources"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/api/volumes"
	apicontext "github.com/docker/compose-cli/context"
	"github.com/docker/compose-cli/context/cloud"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/errdefs"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

const backendType = store.EcsContextType

// ContextParams options for creating AWS context
type ContextParams struct {
	Name         string
	Description  string
	AccessKey    string
	SecretKey    string
	Profile      string
	Region       string
	CredsFromEnv bool
}

func (c ContextParams) haveRequiredEnvVars() bool {
	if c.Profile != "" {
		return true
	}
	if c.AccessKey != "" && c.SecretKey != "" {
		return true
	}
	return false
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
	region := ""
	profile := ecsCtx.Profile

	if ecsCtx.CredentialsFromEnv {
		env := getEnvVars()
		if !env.haveRequiredEnvVars() {
			return nil, fmt.Errorf("context requires credentials to be passed as environment variables")
		}
		profile = env.Profile
		region = env.Region
	}

	if region == "" {
		r, err := getRegion(profile)
		if err != nil {
			return nil, err
		}
		region = r
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Profile:           profile,
		SharedConfigState: session.SharedConfigEnable,
		Config: aws.Config{
			Region: aws.String(region),
		},
	})
	if err != nil {
		return nil, err
	}

	sdk := newSDK(sess)
	return &ecsAPIService{
		ctx:    ecsCtx,
		Region: region,
		aws:    sdk,
	}, nil
}

type ecsAPIService struct {
	ctx    store.EcsContext
	Region string
	aws    API
}

func (b *ecsAPIService) ContainerService() containers.Service {
	return nil
}

func (b *ecsAPIService) ComposeService() compose.Service {
	return b
}

func (b *ecsAPIService) SecretsService() secrets.Service {
	return b
}

func (b *ecsAPIService) VolumeService() volumes.Service {
	return ecsVolumeService{backend: b}
}

func (b *ecsAPIService) ResourceService() resources.Service {
	return nil
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
