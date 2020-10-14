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

package aci

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/aci/convert"
	"github.com/docker/compose-cli/aci/login"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/api/resources"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/api/volumes"
	"github.com/docker/compose-cli/backend"
	apicontext "github.com/docker/compose-cli/context"
	"github.com/docker/compose-cli/context/cloud"
	"github.com/docker/compose-cli/context/store"
)

const (
	backendType               = store.AciContextType
	singleContainerTag        = "docker-single-container"
	composeContainerTag       = "docker-compose-application"
	dockerVolumeTag           = "docker-volume"
	composeContainerSeparator = "_"
)

// LoginParams azure login options
type LoginParams struct {
	TenantID     string
	ClientID     string
	ClientSecret string
}

// Validate returns an error if options are not used properly
func (opts LoginParams) Validate() error {
	if opts.ClientID != "" || opts.ClientSecret != "" {
		if opts.ClientID == "" || opts.ClientSecret == "" || opts.TenantID == "" {
			return errors.New("for Service Principal login, 3 options must be specified: --client-id, --client-secret and --tenant-id")
		}
	}
	return nil
}

func init() {
	backend.Register(backendType, backendType, service, getCloudService)
}

func service(ctx context.Context) (backend.Service, error) {
	contextStore := store.ContextStore(ctx)
	currentContext := apicontext.CurrentContext(ctx)
	var aciContext store.AciContext

	if err := contextStore.GetEndpoint(currentContext, &aciContext); err != nil {
		return nil, err
	}

	return getAciAPIService(aciContext), nil
}

func getCloudService() (cloud.Service, error) {
	service, err := login.NewAzureLoginService()
	if err != nil {
		return nil, err
	}
	return &aciCloudService{
		loginService: service,
	}, nil
}

func getAciAPIService(aciCtx store.AciContext) *aciAPIService {
	containerService := newContainerService(aciCtx)
	composeService := newComposeService(aciCtx)
	return &aciAPIService{
		aciContainerService: &containerService,
		aciComposeService:   &composeService,
		aciVolumeService: &aciVolumeService{
			aciContext: aciCtx,
		},
		aciResourceService: &aciResourceService{
			aciContext: aciCtx,
		},
	}
}

type aciAPIService struct {
	*aciContainerService
	*aciComposeService
	*aciVolumeService
	*aciResourceService
}

func (a *aciAPIService) ContainerService() containers.Service {
	return a.aciContainerService
}

func (a *aciAPIService) ComposeService() compose.Service {
	return a.aciComposeService
}

func (a *aciAPIService) SecretsService() secrets.Service {
	// Not implemented on ACI
	// Secrets are created and mounted in the container at it's creation and not stored on ACI
	return nil
}

func (a *aciAPIService) VolumeService() volumes.Service {
	return a.aciVolumeService
}

func (a *aciAPIService) ResourceService() resources.Service {
	return a.aciResourceService
}

func getContainerID(group containerinstance.ContainerGroup, container containerinstance.Container) string {
	containerID := *group.Name + composeContainerSeparator + *container.Name
	if _, ok := group.Tags[singleContainerTag]; ok {
		containerID = *group.Name
	}
	return containerID
}

func isContainerVisible(container containerinstance.Container, group containerinstance.ContainerGroup, showAll bool) bool {
	return *container.Name == convert.ComposeDNSSidecarName || (!showAll && convert.GetStatus(container, group) != convert.StatusRunning)
}

func addTag(groupDefinition *containerinstance.ContainerGroup, tagName string) {
	if groupDefinition.Tags == nil {
		groupDefinition.Tags = make(map[string]*string, 1)
	}
	groupDefinition.Tags[tagName] = to.StringPtr(tagName)
}

func getGroupAndContainerName(containerID string) (string, string) {
	tokens := strings.Split(containerID, composeContainerSeparator)
	groupName := tokens[0]
	containerName := groupName
	if len(tokens) > 1 {
		containerName = tokens[len(tokens)-1]
		groupName = containerID[:len(containerID)-(len(containerName)+1)]
	}
	return groupName, containerName
}
