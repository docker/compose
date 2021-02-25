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

	"github.com/pkg/errors"

	"github.com/docker/compose-cli/aci/login"
)

type aciCloudService struct {
	loginService login.AzureLoginService
}

func (cs *aciCloudService) Login(ctx context.Context, params interface{}) error {
	opts, ok := params.(LoginParams)
	if !ok {
		return errors.New("could not read Azure LoginParams struct from generic parameter")
	}
	if opts.CloudName == "" {
		opts.CloudName = login.AzurePublicCloudName
	}
	if opts.ClientID != "" {
		return cs.loginService.LoginServicePrincipal(opts.ClientID, opts.ClientSecret, opts.TenantID, opts.CloudName)
	}
	return cs.loginService.Login(ctx, opts.TenantID, opts.CloudName)
}

func (cs *aciCloudService) Logout(ctx context.Context) error {
	return cs.loginService.Logout(ctx)
}

func (cs *aciCloudService) CreateContextData(ctx context.Context, params interface{}) (interface{}, string, error) {
	contextHelper := newContextCreateHelper()
	createOpts := params.(ContextParams)
	return contextHelper.createContextData(ctx, createOpts)
}
