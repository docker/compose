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

package login

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/date"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"

	"github.com/docker/compose-cli/errdefs"
)

//go login process, derived from code sample provided by MS at https://github.com/devigned/go-az-cli-stuff
const (
	authorizeFormat = "https://login.microsoftonline.com/organizations/oauth2/v2.0/authorize?response_type=code&client_id=%s&redirect_uri=%s&state=%s&prompt=select_account&response_mode=query&scope=%s"
	tokenEndpoint   = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
	getTenantURL    = "https://management.azure.com/tenants?api-version=2019-11-01"
	// scopes for a multi-tenant app works for openid, email, other common scopes, but fails when trying to add a token
	// v1 scope like "https://management.azure.com/.default" for ARM access
	scopes   = "offline_access https://management.azure.com/.default"
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
type AzureLoginService struct {
	tokenStore tokenStore
	apiHelper  apiHelper
}

// AzureLoginServiceAPI interface for Azure login service
type AzureLoginServiceAPI interface {
	LoginServicePrincipal(clientID string, clientSecret string, tenantID string) error
	Login(ctx context.Context, requestedTenantID string) error
	Logout(ctx context.Context) error
}

const tokenStoreFilename = "dockerAccessToken.json"

// NewAzureLoginService creates a NewAzureLoginService
func NewAzureLoginService() (*AzureLoginService, error) {
	return newAzureLoginServiceFromPath(GetTokenStorePath(), azureAPIHelper{})
}

func newAzureLoginServiceFromPath(tokenStorePath string, helper apiHelper) (*AzureLoginService, error) {
	store, err := newTokenStore(tokenStorePath)
	if err != nil {
		return nil, err
	}
	return &AzureLoginService{
		tokenStore: store,
		apiHelper:  helper,
	}, nil
}

// LoginServicePrincipal login with clientId / clientSecret from a service principal.
// The resulting token does not include a refresh token
func (login *AzureLoginService) LoginServicePrincipal(clientID string, clientSecret string, tenantID string) error {
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
	loginInfo := TokenInfo{TenantID: tenantID, Token: token}

	if err := login.tokenStore.writeLoginInfo(loginInfo); err != nil {
		return errors.Wrapf(errdefs.ErrLoginFailed, "could not store login info: %s", err)
	}
	return nil
}

// Logout remove azure token data
func (login *AzureLoginService) Logout(ctx context.Context) error {
	err := login.tokenStore.removeData()
	if os.IsNotExist(err) {
		return errors.New("No Azure login data to be removed")
	}
	return err
}

func (login *AzureLoginService) getTenantAndValidateLogin(ctx context.Context, accessToken string, refreshToken string, requestedTenantID string) error {
	bits, statusCode, err := login.apiHelper.queryAPIWithHeader(ctx, getTenantURL, fmt.Sprintf("Bearer %s", accessToken))
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
	tToken, err := login.refreshToken(refreshToken, tenantID)
	if err != nil {
		return errors.Wrapf(errdefs.ErrLoginFailed, "unable to refresh token: %s", err)
	}
	loginInfo := TokenInfo{TenantID: tenantID, Token: tToken}

	if err := login.tokenStore.writeLoginInfo(loginInfo); err != nil {
		return errors.Wrapf(errdefs.ErrLoginFailed, "could not store login info: %s", err)
	}
	return nil
}

// Login performs an Azure login through a web browser
func (login *AzureLoginService) Login(ctx context.Context, requestedTenantID string) error {
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
	if err = login.apiHelper.openAzureLoginPage(redirectURL); err != nil {
		login.startDeviceCodeFlow(deviceCodeFlowCh)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case dcft := <-deviceCodeFlowCh:
		if dcft.err != nil {
			return errors.Wrapf(errdefs.ErrLoginFailed, "could not get token using device code flow: %s", err)
		}
		token := dcft.token
		return login.getTenantAndValidateLogin(ctx, token.AccessToken, token.RefreshToken, requestedTenantID)
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
			"scope":        []string{scopes},
			"redirect_uri": []string{redirectURL},
		}
		token, err := login.apiHelper.queryToken(data, "organizations")
		if err != nil {
			return errors.Wrapf(errdefs.ErrLoginFailed, "access token request failed: %s", err)
		}
		return login.getTenantAndValidateLogin(ctx, token.AccessToken, token.RefreshToken, requestedTenantID)
	}
}

type deviceCodeFlowResponse struct {
	token adal.Token
	err   error
}

func (login *AzureLoginService) startDeviceCodeFlow(deviceCodeFlowCh chan deviceCodeFlowResponse) {
	fmt.Println("Could not automatically open a browser, falling back to Azure device code flow authentication")
	go func() {
		token, err := login.apiHelper.getDeviceCodeFlowToken()
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

// NewAuthorizerFromLogin creates an authorizer based on login access token
func NewAuthorizerFromLogin() (autorest.Authorizer, error) {
	return newAuthorizerFromLoginStorePath(GetTokenStorePath())
}

func newAuthorizerFromLoginStorePath(storeTokenPath string) (autorest.Authorizer, error) {
	login, err := newAzureLoginServiceFromPath(storeTokenPath, azureAPIHelper{})
	if err != nil {
		return nil, err
	}
	oauthToken, err := login.GetValidToken()
	if err != nil {
		return nil, errors.Wrap(err, "not logged in to azure, you need to run \"docker login azure\" first")
	}

	token := adal.Token{
		AccessToken:  oauthToken.AccessToken,
		Type:         oauthToken.TokenType,
		ExpiresIn:    json.Number(strconv.Itoa(int(time.Until(oauthToken.Expiry).Seconds()))),
		ExpiresOn:    json.Number(strconv.Itoa(int(oauthToken.Expiry.Sub(date.UnixEpoch()).Seconds()))),
		RefreshToken: "",
		Resource:     "",
	}

	return autorest.NewBearerAuthorizer(&token), nil
}

// GetTenantID returns tenantID for current login
func (login AzureLoginService) GetTenantID() (string, error) {
	loginInfo, err := login.tokenStore.readToken()
	if err != nil {
		return "", err
	}
	return loginInfo.TenantID, err
}

// GetValidToken returns an access token. Refresh token if needed
func (login *AzureLoginService) GetValidToken() (oauth2.Token, error) {
	loginInfo, err := login.tokenStore.readToken()
	if err != nil {
		return oauth2.Token{}, err
	}
	token := loginInfo.Token
	if token.Valid() {
		return token, nil
	}
	tenantID := loginInfo.TenantID
	token, err = login.refreshToken(token.RefreshToken, tenantID)
	if err != nil {
		return oauth2.Token{}, errors.Wrap(err, "access token request failed. Maybe you need to login to azure again.")
	}
	err = login.tokenStore.writeLoginInfo(TokenInfo{TenantID: tenantID, Token: token})
	if err != nil {
		return oauth2.Token{}, err
	}
	return token, nil
}

func (login *AzureLoginService) refreshToken(currentRefreshToken string, tenantID string) (oauth2.Token, error) {
	data := url.Values{
		"grant_type":    []string{"refresh_token"},
		"client_id":     []string{clientID},
		"scope":         []string{scopes},
		"refresh_token": []string{currentRefreshToken},
	}
	token, err := login.apiHelper.queryToken(data, tenantID)
	if err != nil {
		return oauth2.Token{}, err
	}

	return toOAuthToken(token), nil
}
