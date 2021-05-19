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

package login

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"

	"github.com/stretchr/testify/mock"
	"gotest.tools/v3/assert"

	"golang.org/x/oauth2"
)

func testLoginService(t *testing.T, apiHelperMock *MockAzureHelper, cloudEnvironmentSvc CloudEnvironmentService) (*azureLoginService, error) {
	dir, err := ioutil.TempDir("", "test_store")
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	ces := CloudEnvironments
	if cloudEnvironmentSvc != nil {
		ces = cloudEnvironmentSvc
	}
	return newAzureLoginServiceFromPath(filepath.Join(dir, tokenStoreFilename), apiHelperMock, ces)
}

func TestRefreshInValidToken(t *testing.T) {
	data := url.Values{
		"grant_type":    []string{"refresh_token"},
		"client_id":     []string{clientID},
		"scope":         []string{"offline_access https://management.docker.com/.default"},
		"refresh_token": []string{"refreshToken"},
	}
	helperMock := &MockAzureHelper{}
	helperMock.On("queryToken", mock.AnythingOfType("login.CloudEnvironment"), data, "123456").Return(azureToken{
		RefreshToken: "newRefreshToken",
		AccessToken:  "newAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)

	cloudEnvironmentSvcMock := &MockCloudEnvironmentService{}
	cloudEnvironmentSvcMock.On("Get", "AzureDockerCloud").Return(CloudEnvironment{
		Name: "AzureDockerCloud",
		Authentication: CloudEnvironmentAuthentication{
			LoginEndpoint: "https://login.docker.com",
			Audiences: []string{
				"https://management.docker.com",
				"https://management-ext.docker.com",
			},
			Tenant: "common",
		},
		ResourceManagerURL: "https://management.docker.com",
		Suffixes:           map[string]string{},
	}, nil)

	azureLogin, err := testLoginService(t, helperMock, cloudEnvironmentSvcMock)
	assert.NilError(t, err)
	err = azureLogin.tokenStore.writeLoginInfo(TokenInfo{
		TenantID: "123456",
		Token: oauth2.Token{
			AccessToken:  "accessToken",
			RefreshToken: "refreshToken",
			Expiry:       time.Now().Add(-1 * time.Hour),
			TokenType:    "Bearer",
		},
		CloudEnvironment: "AzureDockerCloud",
	})
	assert.NilError(t, err)

	token, tenantID, err := azureLogin.GetValidToken()
	assert.NilError(t, err)
	assert.Equal(t, tenantID, "123456")

	assert.Equal(t, token.AccessToken, "newAccessToken")
	assert.Assert(t, time.Now().Add(3500*time.Second).Before(token.Expiry))

	storedToken, err := azureLogin.tokenStore.readToken()
	assert.NilError(t, err)
	assert.Equal(t, storedToken.Token.AccessToken, "newAccessToken")
	assert.Equal(t, storedToken.Token.RefreshToken, "newRefreshToken")
	assert.Assert(t, time.Now().Add(3500*time.Second).Before(storedToken.Token.Expiry))

	assert.Equal(t, storedToken.CloudEnvironment, "AzureDockerCloud")
}

func TestDoesNotRefreshValidToken(t *testing.T) {
	expiryDate := time.Now().Add(1 * time.Hour)
	azureLogin, err := testLoginService(t, nil, nil)
	assert.NilError(t, err)
	err = azureLogin.tokenStore.writeLoginInfo(TokenInfo{
		TenantID: "123456",
		Token: oauth2.Token{
			AccessToken:  "accessToken",
			RefreshToken: "refreshToken",
			Expiry:       expiryDate,
			TokenType:    "Bearer",
		},
		CloudEnvironment: AzurePublicCloudName,
	})
	assert.NilError(t, err)

	token, tenantID, err := azureLogin.GetValidToken()
	assert.NilError(t, err)
	assert.Equal(t, token.AccessToken, "accessToken")
	assert.Equal(t, tenantID, "123456")
}

func TestTokenStoreAssumesAzurePublicCloud(t *testing.T) {
	expiryDate := time.Now().Add(1 * time.Hour)
	azureLogin, err := testLoginService(t, nil, nil)
	assert.NilError(t, err)
	err = azureLogin.tokenStore.writeLoginInfo(TokenInfo{
		TenantID: "123456",
		Token: oauth2.Token{
			AccessToken:  "accessToken",
			RefreshToken: "refreshToken",
			Expiry:       expiryDate,
			TokenType:    "Bearer",
		},
		// Simulates upgrade from older version of Docker CLI that did not have cloud environment concept
		CloudEnvironment: "",
	})
	assert.NilError(t, err)

	token, tenantID, err := azureLogin.GetValidToken()
	assert.NilError(t, err)
	assert.Equal(t, tenantID, "123456")
	assert.Equal(t, token.AccessToken, "accessToken")

	ce, err := azureLogin.GetCloudEnvironment()
	assert.NilError(t, err)
	assert.Equal(t, ce.Name, AzurePublicCloudName)
}

func TestInvalidLogin(t *testing.T) {
	m := &MockAzureHelper{}
	m.On("openAzureLoginPage", mock.AnythingOfType("string"), mock.AnythingOfType("CloudEnvironment")).Run(func(args mock.Arguments) {
		redirectURL := args.Get(0).(string)
		err := queryKeyValue(redirectURL, "error", "access denied: login failed")
		assert.NilError(t, err)
	}).Return(nil)

	azureLogin, err := testLoginService(t, m, nil)
	assert.NilError(t, err)

	err = azureLogin.Login(context.TODO(), "", AzurePublicCloudName)
	assert.Error(t, err, "no login code: login failed")
}

func TestValidLogin(t *testing.T) {
	var redirectURL string
	ctx := context.TODO()
	m := &MockAzureHelper{}
	ce, err := CloudEnvironments.Get(AzurePublicCloudName)
	assert.NilError(t, err)

	m.On("openAzureLoginPage", mock.AnythingOfType("string"), mock.AnythingOfType("CloudEnvironment")).Run(func(args mock.Arguments) {
		redirectURL = args.Get(0).(string)
		err := queryKeyValue(redirectURL, "code", "123456879")
		assert.NilError(t, err)
	}).Return(nil)

	m.On("queryToken", mock.AnythingOfType("login.CloudEnvironment"), mock.MatchedBy(func(data url.Values) bool {
		//Need a matcher here because the value of redirectUrl is not known until executing openAzureLoginPage
		return reflect.DeepEqual(data, url.Values{
			"grant_type":   []string{"authorization_code"},
			"client_id":    []string{clientID},
			"code":         []string{"123456879"},
			"scope":        []string{ce.GetTokenScope()},
			"redirect_uri": []string{redirectURL},
		})
	}), "organizations").Return(azureToken{
		RefreshToken: "firstRefreshToken",
		AccessToken:  "firstAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)

	authBody := `{"value":[{"id":"/tenants/12345a7c-c56d-43e8-9549-dd230ce8a038","tenantId":"12345a7c-c56d-43e8-9549-dd230ce8a038"}]}`

	m.On("queryAPIWithHeader", ctx, ce.GetTenantQueryURL(), "Bearer firstAccessToken").Return([]byte(authBody), 200, nil)
	data := refreshTokenData("firstRefreshToken", ce)
	m.On("queryToken", mock.AnythingOfType("login.CloudEnvironment"), data, "12345a7c-c56d-43e8-9549-dd230ce8a038").Return(azureToken{
		RefreshToken: "newRefreshToken",
		AccessToken:  "newAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)
	azureLogin, err := testLoginService(t, m, nil)
	assert.NilError(t, err)

	err = azureLogin.Login(ctx, "", AzurePublicCloudName)
	assert.NilError(t, err)

	loginToken, err := azureLogin.tokenStore.readToken()
	assert.NilError(t, err)
	assert.Equal(t, loginToken.Token.AccessToken, "newAccessToken")
	assert.Equal(t, loginToken.Token.RefreshToken, "newRefreshToken")
	assert.Assert(t, time.Now().Add(3500*time.Second).Before(loginToken.Token.Expiry))
	assert.Equal(t, loginToken.TenantID, "12345a7c-c56d-43e8-9549-dd230ce8a038")
	assert.Equal(t, loginToken.Token.Type(), "Bearer")
	assert.Equal(t, loginToken.CloudEnvironment, "AzureCloud")
}

func TestValidLoginRequestedTenant(t *testing.T) {
	var redirectURL string
	m := &MockAzureHelper{}
	ce, err := CloudEnvironments.Get(AzurePublicCloudName)
	assert.NilError(t, err)

	m.On("openAzureLoginPage", mock.AnythingOfType("string"), mock.AnythingOfType("CloudEnvironment")).Run(func(args mock.Arguments) {
		redirectURL = args.Get(0).(string)
		err := queryKeyValue(redirectURL, "code", "123456879")
		assert.NilError(t, err)
	}).Return(nil)

	m.On("queryToken", mock.AnythingOfType("login.CloudEnvironment"), mock.MatchedBy(func(data url.Values) bool {
		//Need a matcher here because the value of redirectUrl is not known until executing openAzureLoginPage
		return reflect.DeepEqual(data, url.Values{
			"grant_type":   []string{"authorization_code"},
			"client_id":    []string{clientID},
			"code":         []string{"123456879"},
			"scope":        []string{ce.GetTokenScope()},
			"redirect_uri": []string{redirectURL},
		})
	}), "organizations").Return(azureToken{
		RefreshToken: "firstRefreshToken",
		AccessToken:  "firstAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)

	authBody := `{"value":[{"id":"/tenants/00000000-c56d-43e8-9549-dd230ce8a038","tenantId":"00000000-c56d-43e8-9549-dd230ce8a038"},
						   {"id":"/tenants/12345a7c-c56d-43e8-9549-dd230ce8a038","tenantId":"12345a7c-c56d-43e8-9549-dd230ce8a038"}]}`

	ctx := context.TODO()
	m.On("queryAPIWithHeader", ctx, ce.GetTenantQueryURL(), "Bearer firstAccessToken").Return([]byte(authBody), 200, nil)
	data := refreshTokenData("firstRefreshToken", ce)
	m.On("queryToken", mock.AnythingOfType("login.CloudEnvironment"), data, "12345a7c-c56d-43e8-9549-dd230ce8a038").Return(azureToken{
		RefreshToken: "newRefreshToken",
		AccessToken:  "newAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)
	azureLogin, err := testLoginService(t, m, nil)
	assert.NilError(t, err)

	err = azureLogin.Login(ctx, "12345a7c-c56d-43e8-9549-dd230ce8a038", AzurePublicCloudName)
	assert.NilError(t, err)

	loginToken, err := azureLogin.tokenStore.readToken()
	assert.NilError(t, err)
	assert.Equal(t, loginToken.Token.AccessToken, "newAccessToken")
	assert.Equal(t, loginToken.Token.RefreshToken, "newRefreshToken")
	assert.Assert(t, time.Now().Add(3500*time.Second).Before(loginToken.Token.Expiry))
	assert.Equal(t, loginToken.TenantID, "12345a7c-c56d-43e8-9549-dd230ce8a038")
	assert.Equal(t, loginToken.Token.Type(), "Bearer")
	assert.Equal(t, loginToken.CloudEnvironment, "AzureCloud")
}

func TestLoginNoTenant(t *testing.T) {
	var redirectURL string
	m := &MockAzureHelper{}
	ce, err := CloudEnvironments.Get(AzurePublicCloudName)
	assert.NilError(t, err)

	m.On("openAzureLoginPage", mock.AnythingOfType("string"), mock.AnythingOfType("CloudEnvironment")).Run(func(args mock.Arguments) {
		redirectURL = args.Get(0).(string)
		err := queryKeyValue(redirectURL, "code", "123456879")
		assert.NilError(t, err)
	}).Return(nil)

	m.On("queryToken", mock.AnythingOfType("login.CloudEnvironment"), mock.MatchedBy(func(data url.Values) bool {
		//Need a matcher here because the value of redirectUrl is not known until executing openAzureLoginPage
		return reflect.DeepEqual(data, url.Values{
			"grant_type":   []string{"authorization_code"},
			"client_id":    []string{clientID},
			"code":         []string{"123456879"},
			"scope":        []string{ce.GetTokenScope()},
			"redirect_uri": []string{redirectURL},
		})
	}), "organizations").Return(azureToken{
		RefreshToken: "firstRefreshToken",
		AccessToken:  "firstAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)

	ctx := context.TODO()
	authBody := `{"value":[{"id":"/tenants/12345a7c-c56d-43e8-9549-dd230ce8a038","tenantId":"12345a7c-c56d-43e8-9549-dd230ce8a038"}]}`
	m.On("queryAPIWithHeader", ctx, ce.GetTenantQueryURL(), "Bearer firstAccessToken").Return([]byte(authBody), 200, nil)

	azureLogin, err := testLoginService(t, m, nil)
	assert.NilError(t, err)

	err = azureLogin.Login(ctx, "00000000-c56d-43e8-9549-dd230ce8a038", AzurePublicCloudName)
	assert.Error(t, err, "could not find requested azure tenant 00000000-c56d-43e8-9549-dd230ce8a038: login failed")
}

func TestLoginRequestedTenantNotFound(t *testing.T) {
	var redirectURL string
	m := &MockAzureHelper{}
	ce, err := CloudEnvironments.Get(AzurePublicCloudName)
	assert.NilError(t, err)

	m.On("openAzureLoginPage", mock.AnythingOfType("string"), mock.AnythingOfType("CloudEnvironment")).Run(func(args mock.Arguments) {
		redirectURL = args.Get(0).(string)
		err := queryKeyValue(redirectURL, "code", "123456879")
		assert.NilError(t, err)
	}).Return(nil)

	m.On("queryToken", mock.AnythingOfType("login.CloudEnvironment"), mock.MatchedBy(func(data url.Values) bool {
		//Need a matcher here because the value of redirectUrl is not known until executing openAzureLoginPage
		return reflect.DeepEqual(data, url.Values{
			"grant_type":   []string{"authorization_code"},
			"client_id":    []string{clientID},
			"code":         []string{"123456879"},
			"scope":        []string{ce.GetTokenScope()},
			"redirect_uri": []string{redirectURL},
		})
	}), "organizations").Return(azureToken{
		RefreshToken: "firstRefreshToken",
		AccessToken:  "firstAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)

	ctx := context.TODO()
	authBody := `{"value":[]}`
	m.On("queryAPIWithHeader", ctx, ce.GetTenantQueryURL(), "Bearer firstAccessToken").Return([]byte(authBody), 200, nil)

	azureLogin, err := testLoginService(t, m, nil)
	assert.NilError(t, err)

	err = azureLogin.Login(ctx, "", AzurePublicCloudName)
	assert.Error(t, err, "could not find azure tenant: login failed")
}

func TestLoginAuthorizationFailed(t *testing.T) {
	var redirectURL string
	m := &MockAzureHelper{}
	ce, err := CloudEnvironments.Get(AzurePublicCloudName)
	assert.NilError(t, err)

	m.On("openAzureLoginPage", mock.AnythingOfType("string"), mock.AnythingOfType("CloudEnvironment")).Run(func(args mock.Arguments) {
		redirectURL = args.Get(0).(string)
		err := queryKeyValue(redirectURL, "code", "123456879")
		assert.NilError(t, err)
	}).Return(nil)

	m.On("queryToken", mock.AnythingOfType("login.CloudEnvironment"), mock.MatchedBy(func(data url.Values) bool {
		//Need a matcher here because the value of redirectUrl is not known until executing openAzureLoginPage
		return reflect.DeepEqual(data, url.Values{
			"grant_type":   []string{"authorization_code"},
			"client_id":    []string{clientID},
			"code":         []string{"123456879"},
			"scope":        []string{ce.GetTokenScope()},
			"redirect_uri": []string{redirectURL},
		})
	}), "organizations").Return(azureToken{
		RefreshToken: "firstRefreshToken",
		AccessToken:  "firstAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)

	authBody := `[access denied]`

	ctx := context.TODO()
	m.On("queryAPIWithHeader", ctx, ce.GetTenantQueryURL(), "Bearer firstAccessToken").Return([]byte(authBody), 400, nil)

	azureLogin, err := testLoginService(t, m, nil)
	assert.NilError(t, err)

	err = azureLogin.Login(ctx, "", AzurePublicCloudName)
	assert.Error(t, err, "unable to login status code 400: [access denied]: login failed")
}

func TestValidThroughDeviceCodeFlow(t *testing.T) {
	m := &MockAzureHelper{}
	ce, err := CloudEnvironments.Get(AzurePublicCloudName)
	assert.NilError(t, err)

	m.On("openAzureLoginPage", mock.AnythingOfType("string"), mock.AnythingOfType("CloudEnvironment")).Return(errors.New("Could not open browser"))
	m.On("getDeviceCodeFlowToken", mock.AnythingOfType("CloudEnvironment")).Return(adal.Token{AccessToken: "firstAccessToken", RefreshToken: "firstRefreshToken"}, nil)

	authBody := `{"value":[{"id":"/tenants/12345a7c-c56d-43e8-9549-dd230ce8a038","tenantId":"12345a7c-c56d-43e8-9549-dd230ce8a038"}]}`

	ctx := context.TODO()
	m.On("queryAPIWithHeader", ctx, ce.GetTenantQueryURL(), "Bearer firstAccessToken").Return([]byte(authBody), 200, nil)
	data := refreshTokenData("firstRefreshToken", ce)
	m.On("queryToken", mock.AnythingOfType("login.CloudEnvironment"), data, "12345a7c-c56d-43e8-9549-dd230ce8a038").Return(azureToken{
		RefreshToken: "newRefreshToken",
		AccessToken:  "newAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)
	azureLogin, err := testLoginService(t, m, nil)
	assert.NilError(t, err)

	err = azureLogin.Login(ctx, "", AzurePublicCloudName)
	assert.NilError(t, err)

	loginToken, err := azureLogin.tokenStore.readToken()
	assert.NilError(t, err)
	assert.Equal(t, loginToken.Token.AccessToken, "newAccessToken")
	assert.Equal(t, loginToken.Token.RefreshToken, "newRefreshToken")
	assert.Assert(t, time.Now().Add(3500*time.Second).Before(loginToken.Token.Expiry))
	assert.Equal(t, loginToken.TenantID, "12345a7c-c56d-43e8-9549-dd230ce8a038")
	assert.Equal(t, loginToken.Token.Type(), "Bearer")
	assert.Equal(t, loginToken.CloudEnvironment, "AzureCloud")
}

func TestNonstandardCloudEnvironment(t *testing.T) {
	dockerCloudMetadata := []byte(`
	[{
		"authentication": {
			"loginEndpoint": "https://login.docker.com/",
			"audiences": [
				"https://management.docker.com/",
				"https://management.cli.docker.com/"
			],
			"tenant": "F5773994-FE88-482E-9E33-6E799D250416"
		},
		"name": "AzureDockerCloud",
		"suffixes": {
			"acrLoginServer": "azurecr.docker.io"
		},
		"resourceManager": "https://management.docker.com/"
	}]`)
	var metadataReqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write(dockerCloudMetadata)
		assert.NilError(t, err)
		atomic.AddInt32(&metadataReqCount, 1)
	}))
	defer srv.Close()

	cloudMetadataURL, cloudMetadataURLSet := os.LookupEnv(CloudMetadataURLVar)
	if cloudMetadataURLSet {
		defer func() {
			err := os.Setenv(CloudMetadataURLVar, cloudMetadataURL)
			assert.NilError(t, err)
		}()
	}
	err := os.Setenv(CloudMetadataURLVar, srv.URL)
	assert.NilError(t, err)

	ctx := context.TODO()

	ces := newCloudEnvironmentService()
	ces.cloudMetadataURL = srv.URL
	dockerCloudEnv, err := ces.Get("AzureDockerCloud")
	assert.NilError(t, err)

	helperMock := &MockAzureHelper{}
	var redirectURL string
	helperMock.On("openAzureLoginPage", mock.AnythingOfType("string"), mock.AnythingOfType("CloudEnvironment")).Run(func(args mock.Arguments) {
		redirectURL = args.Get(0).(string)
		err := queryKeyValue(redirectURL, "code", "123456879")
		assert.NilError(t, err)
	}).Return(nil)

	helperMock.On("queryToken", mock.AnythingOfType("login.CloudEnvironment"), mock.MatchedBy(func(data url.Values) bool {
		//Need a matcher here because the value of redirectUrl is not known until executing openAzureLoginPage
		return reflect.DeepEqual(data, url.Values{
			"grant_type":   []string{"authorization_code"},
			"client_id":    []string{clientID},
			"code":         []string{"123456879"},
			"scope":        []string{dockerCloudEnv.GetTokenScope()},
			"redirect_uri": []string{redirectURL},
		})
	}), "organizations").Return(azureToken{
		RefreshToken: "firstRefreshToken",
		AccessToken:  "firstAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)

	authBody := `{"value":[{"id":"/tenants/F5773994-FE88-482E-9E33-6E799D250416","tenantId":"F5773994-FE88-482E-9E33-6E799D250416"}]}`

	helperMock.On("queryAPIWithHeader", ctx, dockerCloudEnv.GetTenantQueryURL(), "Bearer firstAccessToken").Return([]byte(authBody), 200, nil)
	data := refreshTokenData("firstRefreshToken", dockerCloudEnv)
	helperMock.On("queryToken", mock.AnythingOfType("login.CloudEnvironment"), data, "F5773994-FE88-482E-9E33-6E799D250416").Return(azureToken{
		RefreshToken: "newRefreshToken",
		AccessToken:  "newAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)

	azureLogin, err := testLoginService(t, helperMock, ces)
	assert.NilError(t, err)

	err = azureLogin.Login(ctx, "", "AzureDockerCloud")
	assert.NilError(t, err)

	loginToken, err := azureLogin.tokenStore.readToken()
	assert.NilError(t, err)
	assert.Equal(t, loginToken.Token.AccessToken, "newAccessToken")
	assert.Equal(t, loginToken.Token.RefreshToken, "newRefreshToken")
	assert.Assert(t, time.Now().Add(3500*time.Second).Before(loginToken.Token.Expiry))
	assert.Equal(t, loginToken.TenantID, "F5773994-FE88-482E-9E33-6E799D250416")
	assert.Equal(t, loginToken.Token.Type(), "Bearer")
	assert.Equal(t, loginToken.CloudEnvironment, "AzureDockerCloud")
	assert.Equal(t, metadataReqCount, int32(1))
}

// Don't warn about refreshToken parameter taking the same value for all invocations
// nolint:unparam
func refreshTokenData(refreshToken string, ce CloudEnvironment) url.Values {
	return url.Values{
		"grant_type":    []string{"refresh_token"},
		"client_id":     []string{clientID},
		"scope":         []string{ce.GetTokenScope()},
		"refresh_token": []string{refreshToken},
	}
}

func queryKeyValue(redirectURL string, key string, value string) error {
	req, err := http.NewRequest("GET", redirectURL, nil)
	if err != nil {
		return err
	}
	q := req.URL.Query()
	q.Add(key, value)
	req.URL.RawQuery = q.Encode()
	client := &http.Client{}
	_, err = client.Do(req)
	return err
}

type MockAzureHelper struct {
	mock.Mock
}

func (s *MockAzureHelper) getDeviceCodeFlowToken(ce CloudEnvironment) (adal.Token, error) {
	args := s.Called(ce)
	return args.Get(0).(adal.Token), args.Error(1)
}

func (s *MockAzureHelper) queryToken(ce CloudEnvironment, data url.Values, tenantID string) (token azureToken, err error) {
	args := s.Called(ce, data, tenantID)
	return args.Get(0).(azureToken), args.Error(1)
}

func (s *MockAzureHelper) queryAPIWithHeader(ctx context.Context, authorizationURL string, authorizationHeader string) ([]byte, int, error) {
	args := s.Called(ctx, authorizationURL, authorizationHeader)
	return args.Get(0).([]byte), args.Int(1), args.Error(2)
}

func (s *MockAzureHelper) openAzureLoginPage(redirectURL string, ce CloudEnvironment) error {
	args := s.Called(redirectURL, ce)
	return args.Error(0)
}

type MockCloudEnvironmentService struct {
	mock.Mock
}

func (s *MockCloudEnvironmentService) Get(name string) (CloudEnvironment, error) {
	args := s.Called(name)
	return args.Get(0).(CloudEnvironment), args.Error(1)
}
