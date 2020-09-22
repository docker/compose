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

package cloud

import (
	"context"

	"github.com/docker/compose-cli/errdefs"
)

// Service cloud specific services
type Service interface {
	// Login login to cloud provider
	Login(ctx context.Context, params interface{}) error
	// Logout logout from cloud provider
	Logout(ctx context.Context) error
	// CreateContextData create data for cloud context
	CreateContextData(ctx context.Context, params interface{}) (contextData interface{}, description string, err error)
}

// NotImplementedCloudService to use for backend that don't provide cloud services
func NotImplementedCloudService() (Service, error) {
	return notImplementedCloudService{}, nil
}

type notImplementedCloudService struct {
}

// Logout login to cloud provider
func (cs notImplementedCloudService) Logout(ctx context.Context) error {
	return errdefs.ErrNotImplemented
}

func (cs notImplementedCloudService) Login(ctx context.Context, params interface{}) error {
	return errdefs.ErrNotImplemented
}

func (cs notImplementedCloudService) CreateContextData(ctx context.Context, params interface{}) (interface{}, string, error) {
	return nil, "", errdefs.ErrNotImplemented
}
