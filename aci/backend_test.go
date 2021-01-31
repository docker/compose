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
	"testing"

	"github.com/stretchr/testify/mock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose-cli/aci/login"
	"github.com/docker/compose-cli/api/containers"
	"golang.org/x/oauth2"
)

func TestGetContainerName(t *testing.T) {
	group, container := getGroupAndContainerName("docker1234")
	assert.Equal(t, group, "docker1234")
	assert.Equal(t, container, "docker1234")

	group, container = getGroupAndContainerName("compose_service1")
	assert.Equal(t, group, "compose")
	assert.Equal(t, container, "service1")

	group, container = getGroupAndContainerName("compose_stack_service1")
	assert.Equal(t, group, "compose_stack")
	assert.Equal(t, container, "service1")
}

func TestErrorMessageDeletingContainerFromComposeApplication(t *testing.T) {
	service := aciContainerService{}
	err := service.Delete(context.TODO(), "compose-app_service1", containers.DeleteRequest{Force: false})
	assert.Error(t, err, "cannot delete service \"service1\" from compose application \"compose-app\", you can delete the entire compose app with docker compose down --project-name compose-app")
}

func TestErrorMessageRunSingleContainerNameWithComposeSeparator(t *testing.T) {
	service := aciContainerService{}
	err := service.Run(context.TODO(), containers.ContainerConfig{ID: "container_name"})
	assert.Error(t, err, "invalid container name. ACI container name cannot include \"_\"")
}

func TestVerifyCommand(t *testing.T) {
	err := verifyExecCommand("command") // Command without an argument
	assert.NilError(t, err)
	err = verifyExecCommand("command argument") // Command with argument
	assert.Error(t, err, "ACI exec command does not accept arguments to the command. "+
		"Only the binary should be specified")
}

func TestLoginParamsValidate(t *testing.T) {
	err := LoginParams{
		ClientID: "someID",
	}.Validate()
	assert.Error(t, err, "for Service Principal login, 3 options must be specified: --client-id, --client-secret and --tenant-id")

	err = LoginParams{
		ClientSecret: "someSecret",
	}.Validate()
	assert.Error(t, err, "for Service Principal login, 3 options must be specified: --client-id, --client-secret and --tenant-id")

	err = LoginParams{}.Validate()
	assert.NilError(t, err)

	err = LoginParams{
		TenantID: "tenant",
	}.Validate()
	assert.NilError(t, err)
}

func TestLoginServicePrincipal(t *testing.T) {
	loginService := mockLoginService{}
	loginService.On("LoginServicePrincipal", "someID", "secret", "tenant", "someCloud").Return(nil)
	loginBackend := aciCloudService{
		loginService: &loginService,
	}

	err := loginBackend.Login(context.Background(), LoginParams{
		ClientID:     "someID",
		ClientSecret: "secret",
		TenantID:     "tenant",
		CloudName:    "someCloud",
	})

	assert.NilError(t, err)
}

func TestLoginWithTenant(t *testing.T) {
	loginService := mockLoginService{}
	ctx := context.Background()
	loginService.On("Login", ctx, "tenant", "someCloud").Return(nil)
	loginBackend := aciCloudService{
		loginService: &loginService,
	}

	err := loginBackend.Login(ctx, LoginParams{
		TenantID:  "tenant",
		CloudName: "someCloud",
	})

	assert.NilError(t, err)
}

func TestLoginWithoutTenant(t *testing.T) {
	loginService := mockLoginService{}
	ctx := context.Background()
	loginService.On("Login", ctx, "", "someCloud").Return(nil)
	loginBackend := aciCloudService{
		loginService: &loginService,
	}

	err := loginBackend.Login(ctx, LoginParams{
		CloudName: "someCloud",
	})

	assert.NilError(t, err)
}

type mockLoginService struct {
	mock.Mock
}

func (s *mockLoginService) Login(ctx context.Context, requestedTenantID string, cloudEnvironment string) error {
	args := s.Called(ctx, requestedTenantID, cloudEnvironment)
	return args.Error(0)
}

func (s *mockLoginService) LoginServicePrincipal(clientID string, clientSecret string, tenantID string, cloudEnvironment string) error {
	args := s.Called(clientID, clientSecret, tenantID, cloudEnvironment)
	return args.Error(0)
}

func (s *mockLoginService) Logout(ctx context.Context) error {
	args := s.Called(ctx)
	return args.Error(0)
}

func (s *mockLoginService) GetTenantID() (string, error) {
	args := s.Called()
	return args.String(0), args.Error(1)
}

func (s *mockLoginService) GetCloudEnvironment() (login.CloudEnvironment, error) {
	args := s.Called()
	return args.Get(0).(login.CloudEnvironment), args.Error(1)
}

func (s *mockLoginService) GetValidToken() (oauth2.Token, string, error) {
	args := s.Called()
	return args.Get(0).(oauth2.Token), args.String(1), args.Error(2)
}
