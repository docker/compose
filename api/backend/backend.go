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

package backend

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/api/resources"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/api/volumes"
	"github.com/docker/compose-cli/context/cloud"
	"github.com/docker/compose-cli/errdefs"
)

var (
	errNoType         = errors.New("backend: no type")
	errNoName         = errors.New("backend: no name")
	errTypeRegistered = errors.New("backend: already registered")
)

type initFunc func(context.Context) (Service, error)
type getCloudServiceFunc func() (cloud.Service, error)

type registeredBackend struct {
	name            string
	backendType     string
	init            initFunc
	getCloudService getCloudServiceFunc
}

var backends = struct {
	r []*registeredBackend
}{}

// Service aggregates the service interfaces
type Service interface {
	ContainerService() containers.Service
	ComposeService() compose.Service
	ResourceService() resources.Service
	SecretsService() secrets.Service
	VolumeService() volumes.Service
}

// Register adds a typed backend to the registry
func Register(name string, backendType string, init initFunc, getCoudService getCloudServiceFunc) {
	if name == "" {
		logrus.Fatal(errNoName)
	}
	if backendType == "" {
		logrus.Fatal(errNoType)
	}
	for _, b := range backends.r {
		if b.backendType == backendType {
			logrus.Fatal(errTypeRegistered)
		}
	}

	backends.r = append(backends.r, &registeredBackend{
		name,
		backendType,
		init,
		getCoudService,
	})
}

// Get returns the backend registered for a particular type, it returns
// an error if there is no registered backends for the given type.
func Get(ctx context.Context, backendType string) (Service, error) {
	for _, b := range backends.r {
		if b.backendType == backendType {
			return b.init(ctx)
		}
	}

	return nil, errdefs.ErrNotFound
}

// GetCloudService returns the backend registered for a particular type, it returns
// an error if there is no registered backends for the given type.
func GetCloudService(ctx context.Context, backendType string) (cloud.Service, error) {
	for _, b := range backends.r {
		if b.backendType == backendType {
			return b.getCloudService()
		}
	}

	return nil, fmt.Errorf("backend not found for backend type %s", backendType)
}
