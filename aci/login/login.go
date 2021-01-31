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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"

	"github.com/docker/compose-cli/api/errdefs"
)

//go login process, derived from code sample provided by MS at https://github.com/devigned/go-az-cli-stuff
const (
	clientID = "04b07795-8ddb-461a-bbee-02f9e1bf7b46" // Azure CLI client id
)

type (
	azureToken struct {
		Type         string `json:"token_type"`
		Scope        string `json:"scope"`
		ExpiresIn    int    `json:"expires_in"`
		ExtExpiresIn int    `json:"ext_expires_in"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Foci         string `json:"foci"`
	}

	tenantResult struct {
		Value []tenantValue `json:"value"`
	}
	tenantValue struct {
		TenantID string `json:"tenantId"`
	}
)

// AzureLoginService Service to log into azure and get authentifier for azure APIs
type AzureLoginService interface {
	Login(ctx context.Context, requestedTenantID string, cloudEnvironment string) error
	LoginServicePrincipal(clientID string, clientSecret string, tenantID string, cloudEnvironment string) error
	Logout(ctx context.Context) error
	GetCloudEnvironment() (CloudEnvironment, error)
	GetValidToken() (oauth2.Token, string, error)
}
type azureLoginService struct {
	tokenStore          tokenStore
	apiHelper           apiHelper
	cloudEnvironmentSvc CloudEnvironmentService
}

const tokenStoreFilename = "dockerAccessToken.json"

// NewAzureLoginService creates a NewAzureLoginService
func NewAzureLoginService() (AzureLoginService, error) {
	return newAzureLoginServiceFromPath(GetTokenStorePath(), azureAPIHelper{}, CloudEnvironments)
}

func newAzureLoginServiceFromPath(tokenStorePath string, helper apiHelper, ces CloudEnvironmentService) (*azureLoginService, error) {
	store, err := newTokenStore(tokenStorePath)
	if err != nil {
		return nil, err
	}
	return &azureLoginService{
		tokenStore:          store,
		apiHelper:           helper,
		cloudEnvironmentSvc: ces,
	}, nil
}

// LoginServicePrincipal login with clientId / clientSecret from a service principal.
// The resulting token does not include a refresh token
func (login *azureLoginService) LoginServicePrincipal(clientID string, clientSecret string, tenantID string, cloudEnvironment string) error {
	// Tried with auth2.NewUsernamePasswordConfig() but could not make this work with username / password, setting this for CI with clientID / clientSecret
	creds := auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID)

	spToken, err := creds.ServicePrincipalToken()
	if err != nil {
		return errors.Wrapf(errdefs.ErrLoginFailed, "could not login with service principal: %s", err)
	}
	err = spToken.Refresh()
	if err != nil {
		return errors.Wrapf(errdefs.ErrLoginFailed, "could not login with service principal: %s", err)
	}
	token, err := spToOAuthToken(spToken.Token())
	if err != nil {
		return errors.Wrapf(errdefs.ErrLoginFailed, "could not read service principal token expiry: %s", err)
	}
	loginInfo := TokenInfo{TenantID: tenantID, Token: token, CloudEnvironment: cloudEnvironment}

	if err := login.tokenStore.writeLoginInfo(loginInfo); err != nil {
		return errors.Wrapf(errdefs.ErrLoginFailed, "could not store login info: %s", err)
	}
	return nil
}

// Logout remove azure token data
func (login *azureLoginService) Logout(ctx context.Context) error {
	err := login.tokenStore.removeData()
	if os.IsNotExist(err) {
		return errors.New("No Azure login data to be removed")
	}
	return err
}

func (login *azureLoginService) getTenantAndValidateLogin(
	ctx context.Context,
	accessToken string,
	refreshToken string,
	requestedTenantID string,
	ce CloudEnvironment,
) error {
	bits, statusCode, err := login.apiHelper.queryAPIWithHeader(ctx, ce.GetTenantQueryURL(), fmt.Sprintf("Bearer %s", accessToken))
	if err != nil {
		return errors.Wrapf(errdefs.ErrLoginFailed, "check auth failed: %s", err)
	}

	if statusCode != http.StatusOK {
		return errors.Wrapf(errdefs.ErrLoginFailed, "unable to login status code %d: %s", statusCode, bits)
	}
	var t tenantResult
	if err := json.Unmarshal(bits, &t); err != nil {
		return errors.Wrapf(errdefs.ErrLoginFailed, "unable to unmarshal tenant: %s", err)
	}
	tenantID, err := getTenantID(t.Value, requestedTenantID)
	if err != nil {
		return errors.Wrap(errdefs.ErrLoginFailed, err.Error())
	}
	tToken, err := login.refreshToken(refreshToken, tenantID, ce)
	if err != nil {
		return errors.Wrapf(errdefs.ErrLoginFailed, "unable to refresh token: %s", err)
	}
	loginInfo := TokenInfo{TenantID: tenantID, Token: tToken, CloudEnvironment: ce.Name}

	if err := login.tokenStore.writeLoginInfo(loginInfo); err != nil {
		return errors.Wrapf(errdefs.ErrLoginFailed, "could not store login info: %s", err)
	}
	return nil
}

// Login performs an Azure login through a web browser
func (login *azureLoginService) Login(ctx context.Context, requestedTenantID string, cloudEnvironment string) error {
	ce, err := login.cloudEnvironmentSvc.Get(cloudEnvironment)
	if err != nil {
		return err
	}

	queryCh := make(chan localResponse, 1)
	s, err := NewLocalServer(queryCh)
	if err != nil {
		return err
	}
	s.Serve()
	defer s.Close()

	redirectURL := s.Addr()
	if redirectURL == "" {
		return errors.Wrap(errdefs.ErrLoginFailed, "empty redirect URL")
	}

	deviceCodeFlowCh := make(chan deviceCodeFlowResponse, 1)
	if err = login.apiHelper.openAzureLoginPage(redirectURL, ce); err != nil {
		login.startDeviceCodeFlow(deviceCodeFlowCh, ce)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case dcft := <-deviceCodeFlowCh:
		if dcft.err != nil {
			return errors.Wrapf(errdefs.ErrLoginFailed, "could not get token using device code flow: %s", err)
		}
		token := dcft.token
		return login.getTenantAndValidateLogin(ctx, token.AccessToken, token.RefreshToken, requestedTenantID, ce)
	case q := <-queryCh:
		if q.err != nil {
			return errors.Wrapf(errdefs.ErrLoginFailed, "unhandled local login server error: %s", err)
		}
		code, hasCode := q.values["code"]
		if !hasCode {
			return errors.Wrap(errdefs.ErrLoginFailed, "no login code")
		}
		data := url.Values{
			"grant_type":   []string{"authorization_code"},
			"client_id":    []string{clientID},
			"code":         code,
			"scope":        []string{ce.GetTokenScope()},
			"redirect_uri": []string{redirectURL},
		}
		token, err := login.apiHelper.queryToken(ce, data, "organizations")
		if err != nil {
			return errors.Wrapf(errdefs.ErrLoginFailed, "access token request failed: %s", err)
		}
		return login.getTenantAndValidateLogin(ctx, token.AccessToken, token.RefreshToken, requestedTenantID, ce)
	}
}

type deviceCodeFlowResponse struct {
	token adal.Token
	err   error
}

func (login *azureLoginService) startDeviceCodeFlow(deviceCodeFlowCh chan deviceCodeFlowResponse, ce CloudEnvironment) {
	fmt.Println("Could not automatically open a browser, falling back to Azure device code flow authentication")
	go func() {
		token, err := login.apiHelper.getDeviceCodeFlowToken(ce)
		if err != nil {
			deviceCodeFlowCh <- deviceCodeFlowResponse{err: err}
		}
		deviceCodeFlowCh <- deviceCodeFlowResponse{token: token}
	}()
}

func getTenantID(tenantValues []tenantValue, requestedTenantID string) (string, error) {
	if requestedTenantID == "" {
		if len(tenantValues) < 1 {
			return "", errors.Errorf("could not find azure tenant")
		}
		return tenantValues[0].TenantID, nil
	}
	for _, tValue := range tenantValues {
		if tValue.TenantID == requestedTenantID {
			return tValue.TenantID, nil
		}
	}
	return "", errors.Errorf("could not find requested azure tenant %s", requestedTenantID)
}

func toOAuthToken(token azureToken) oauth2.Token {
	expireTime := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	oauthToken := oauth2.Token{
		RefreshToken: token.RefreshToken,
		AccessToken:  token.AccessToken,
		Expiry:       expireTime,
		TokenType:    token.Type,
	}
	return oauthToken
}

func spToOAuthToken(token adal.Token) (oauth2.Token, error) {
	expiresIn, err := token.ExpiresIn.Int64()
	if err != nil {
		return oauth2.Token{}, err
	}
	expireTime := time.Now().Add(time.Duration(expiresIn) * time.Second)
	oauthToken := oauth2.Token{
		RefreshToken: token.RefreshToken,
		AccessToken:  token.AccessToken,
		Expiry:       expireTime,
		TokenType:    token.Type,
	}
	return oauthToken, nil
}

// GetValidToken returns an access token and associated tenant ID.
// Will refresh the token as necessary.
func (login *azureLoginService) GetValidToken() (oauth2.Token, string, error) {
	loginInfo, err := login.tokenStore.readToken()
	if err != nil {
		return oauth2.Token{}, "", err
	}
	token := loginInfo.Token
	tenantID := loginInfo.TenantID
	if token.Valid() {
		return token, tenantID, nil
	}

	ce, err := login.cloudEnvironmentSvc.Get(loginInfo.CloudEnvironment)
	if err != nil {
		return oauth2.Token{}, "", errors.Wrap(err, "access token request failed--cloud environment could not be determined.")
	}

	token, err = login.refreshToken(token.RefreshToken, tenantID, ce)
	if err != nil {
		return oauth2.Token{}, "", errors.Wrap(err, "access token request failed. Maybe you need to login to Azure again.")
	}
	err = login.tokenStore.writeLoginInfo(TokenInfo{TenantID: tenantID, Token: token, CloudEnvironment: ce.Name})
	if err != nil {
		return oauth2.Token{}, "", err
	}
	return token, tenantID, nil
}

// GeCloudEnvironment returns the cloud environment associated with the current authentication token (if we have one)
func (login *azureLoginService) GetCloudEnvironment() (CloudEnvironment, error) {
	tokenInfo, err := login.tokenStore.readToken()
	if err != nil {
		return CloudEnvironment{}, err
	}

	cloudEnvironment, err := login.cloudEnvironmentSvc.Get(tokenInfo.CloudEnvironment)
	if err != nil {
		return CloudEnvironment{}, err
	}

	return cloudEnvironment, nil
}

func (login *azureLoginService) refreshToken(currentRefreshToken string, tenantID string, ce CloudEnvironment) (oauth2.Token, error) {
	data := url.Values{
		"grant_type":    []string{"refresh_token"},
		"client_id":     []string{clientID},
		"scope":         []string{ce.GetTokenScope()},
		"refresh_token": []string{currentRefreshToken},
	}
	token, err := login.apiHelper.queryToken(ce, data, tenantID)
	if err != nil {
		return oauth2.Token{}, err
	}

	return toOAuthToken(token), nil
}
