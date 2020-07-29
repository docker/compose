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

package convert

import (
	"strconv"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"
	cliconfigtypes "github.com/docker/cli/cli/config/types"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerinstance/mgmt/containerinstance"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const getAllCredentials = "getAllRegistryCredentials"

type RegistryConvertTestSuite struct {
	suite.Suite
	loader *MockRegistryLoader
}

func (suite *RegistryConvertTestSuite) BeforeTest(suiteName, testName string) {
	suite.loader = &MockRegistryLoader{}
}

func (suite *RegistryConvertTestSuite) TestHubPrivateImage() {
	suite.loader.On(getAllCredentials).Return(registry("https://index.docker.io", userPwdCreds("toto", "pwd")), nil)

	creds, err := getRegistryCredentials(composeServices("gtardif/privateimg"), suite.loader)
	Expect(err).To(BeNil())
	Expect(creds).To(Equal([]containerinstance.ImageRegistryCredential{
		{
			Server:   to.StringPtr(dockerHub),
			Username: to.StringPtr("toto"),
			Password: to.StringPtr("pwd"),
		},
	}))
}

func (suite *RegistryConvertTestSuite) TestRegistryNameWithoutProtocol() {
	suite.loader.On(getAllCredentials).Return(registry("index.docker.io", userPwdCreds("toto", "pwd")), nil)

	creds, err := getRegistryCredentials(composeServices("gtardif/privateimg"), suite.loader)
	Expect(err).To(BeNil())
	Expect(creds).To(Equal([]containerinstance.ImageRegistryCredential{
		{
			Server:   to.StringPtr(dockerHub),
			Username: to.StringPtr("toto"),
			Password: to.StringPtr("pwd"),
		},
	}))
}

func (suite *RegistryConvertTestSuite) TestImageWithDotInName() {
	suite.loader.On(getAllCredentials).Return(registry("index.docker.io", userPwdCreds("toto", "pwd")), nil)

	creds, err := getRegistryCredentials(composeServices("my.image"), suite.loader)
	Expect(err).To(BeNil())
	Expect(creds).To(Equal([]containerinstance.ImageRegistryCredential{
		{
			Server:   to.StringPtr(dockerHub),
			Username: to.StringPtr("toto"),
			Password: to.StringPtr("pwd"),
		},
	}))
}

func (suite *RegistryConvertTestSuite) TestAcrPrivateImage() {
	suite.loader.On(getAllCredentials).Return(registry("https://mycontainerregistrygta.azurecr.io", tokenCreds("123456")), nil)

	creds, err := getRegistryCredentials(composeServices("mycontainerregistrygta.azurecr.io/privateimg"), suite.loader)
	Expect(err).To(BeNil())
	Expect(creds).To(Equal([]containerinstance.ImageRegistryCredential{
		{
			Server:   to.StringPtr("mycontainerregistrygta.azurecr.io"),
			Username: to.StringPtr(tokenUsername),
			Password: to.StringPtr("123456"),
		},
	}))
}

func (suite *RegistryConvertTestSuite) TestAcrPrivateImageLinux() {
	token := tokenCreds("123456")
	token.Username = tokenUsername
	suite.loader.On(getAllCredentials).Return(registry("https://mycontainerregistrygta.azurecr.io", token), nil)

	creds, err := getRegistryCredentials(composeServices("mycontainerregistrygta.azurecr.io/privateimg"), suite.loader)
	Expect(err).To(BeNil())
	Expect(creds).To(Equal([]containerinstance.ImageRegistryCredential{
		{
			Server:   to.StringPtr("mycontainerregistrygta.azurecr.io"),
			Username: to.StringPtr(tokenUsername),
			Password: to.StringPtr("123456"),
		},
	}))
}

func (suite *RegistryConvertTestSuite) TestNoMoreRegistriesThanImages() {
	configs := map[string]cliconfigtypes.AuthConfig{
		"https://mycontainerregistrygta.azurecr.io": tokenCreds("123456"),
		"https://index.docker.io":                   userPwdCreds("toto", "pwd"),
	}
	suite.loader.On(getAllCredentials).Return(configs, nil)

	creds, err := getRegistryCredentials(composeServices("mycontainerregistrygta.azurecr.io/privateimg"), suite.loader)
	Expect(err).To(BeNil())
	Expect(creds).To(Equal([]containerinstance.ImageRegistryCredential{
		{
			Server:   to.StringPtr("mycontainerregistrygta.azurecr.io"),
			Username: to.StringPtr(tokenUsername),
			Password: to.StringPtr("123456"),
		},
	}))

	creds, err = getRegistryCredentials(composeServices("someuser/privateimg"), suite.loader)
	Expect(err).To(BeNil())
	Expect(creds).To(Equal([]containerinstance.ImageRegistryCredential{
		{
			Server:   to.StringPtr(dockerHub),
			Username: to.StringPtr("toto"),
			Password: to.StringPtr("pwd"),
		},
	}))
}

func (suite *RegistryConvertTestSuite) TestHubAndSeveralACRRegistries() {
	configs := map[string]cliconfigtypes.AuthConfig{
		"https://mycontainerregistry1.azurecr.io": tokenCreds("123456"),
		"https://mycontainerregistry2.azurecr.io": tokenCreds("456789"),
		"https://mycontainerregistry3.azurecr.io": tokenCreds("123456789"),
		"https://index.docker.io":                 userPwdCreds("toto", "pwd"),
		"https://other.registry.io":               userPwdCreds("user", "password"),
	}
	suite.loader.On(getAllCredentials).Return(configs, nil)

	creds, err := getRegistryCredentials(composeServices("mycontainerregistry1.azurecr.io/privateimg", "someuser/privateImg2", "mycontainerregistry2.azurecr.io/privateimg"), suite.loader)
	Expect(err).To(BeNil())
	Expect(creds).To(ContainElement(containerinstance.ImageRegistryCredential{
		Server:   to.StringPtr("mycontainerregistry1.azurecr.io"),
		Username: to.StringPtr(tokenUsername),
		Password: to.StringPtr("123456"),
	}))
	Expect(creds).To(ContainElement(containerinstance.ImageRegistryCredential{
		Server:   to.StringPtr("mycontainerregistry2.azurecr.io"),
		Username: to.StringPtr(tokenUsername),
		Password: to.StringPtr("456789"),
	}))
	Expect(creds).To(ContainElement(containerinstance.ImageRegistryCredential{
		Server:   to.StringPtr(dockerHub),
		Username: to.StringPtr("toto"),
		Password: to.StringPtr("pwd"),
	}))
}

func composeServices(images ...string) types.Project {
	var services []types.ServiceConfig
	for index, name := range images {
		service := types.ServiceConfig{
			Name:  "service" + strconv.Itoa(index),
			Image: name,
		}
		services = append(services, service)
	}
	return types.Project{
		Services: services,
	}
}

func registry(host string, configregistryData cliconfigtypes.AuthConfig) map[string]cliconfigtypes.AuthConfig {
	return map[string]cliconfigtypes.AuthConfig{
		host: configregistryData,
	}
}

func userPwdCreds(user string, password string) cliconfigtypes.AuthConfig {
	return cliconfigtypes.AuthConfig{
		Username: user,
		Password: password,
	}
}

func tokenCreds(token string) cliconfigtypes.AuthConfig {
	return cliconfigtypes.AuthConfig{
		IdentityToken: token,
	}
}

func TestRegistryConvertTestSuite(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(RegistryConvertTestSuite))
}

type MockRegistryLoader struct {
	mock.Mock
}

func (s *MockRegistryLoader) getAllRegistryCredentials() (map[string]cliconfigtypes.AuthConfig, error) {
	args := s.Called()
	return args.Get(0).(map[string]cliconfigtypes.AuthConfig), args.Error(1)
}
