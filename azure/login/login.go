package login

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/docker/api/errdefs"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure/cli"
	"github.com/Azure/go-autorest/autorest/date"
	"golang.org/x/oauth2"

	"github.com/pkg/errors"
)

func init() {
	rand.Seed(time.Now().Unix())
}

//go login process, derived from code sample provided by MS at https://github.com/devigned/go-az-cli-stuff
const (
	authorizeFormat = "https://login.microsoftonline.com/organizations/oauth2/v2.0/authorize?response_type=code&client_id=%s&redirect_uri=%s&state=%s&prompt=select_account&response_mode=query&scope=%s"
	tokenEndpoint   = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
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

const tokenStoreFilename = "dockerAccessToken.json"

func getTokenStorePath() string {
	cliPath, _ := cli.AccessTokensPath()
	return filepath.Join(filepath.Dir(cliPath), tokenStoreFilename)
}

// NewAzureLoginService creates a NewAzureLoginService
func NewAzureLoginService() (AzureLoginService, error) {
	return newAzureLoginServiceFromPath(getTokenStorePath(), azureAPIHelper{})
}

func newAzureLoginServiceFromPath(tokenStorePath string, helper apiHelper) (AzureLoginService, error) {
	store, err := newTokenStore(tokenStorePath)
	if err != nil {
		return AzureLoginService{}, err
	}
	return AzureLoginService{
		tokenStore: store,
		apiHelper:  helper,
	}, nil
}

type apiHelper interface {
	queryToken(data url.Values, tenantID string) (azureToken, error)
}

type azureAPIHelper struct{}

//Login perform azure login through browser
func (login AzureLoginService) Login(ctx context.Context) error {
	queryCh := make(chan url.Values, 1)
	serverPort, err := startLoginServer(queryCh)
	if err != nil {
		return err
	}

	redirectURL := "http://localhost:" + strconv.Itoa(serverPort)
	openAzureLoginPage(redirectURL)

	select {
	case <-ctx.Done():
		return nil
	case qsValues := <-queryCh:
		errorMsg, hasError := qsValues["error"]
		if hasError {
			return fmt.Errorf("login failed : %s", errorMsg)
		}
		code, hasCode := qsValues["code"]
		if !hasCode {
			return errdefs.ErrLoginFailed
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
			return errors.Wrap(err, "Access token request failed")
		}

		req, err := http.NewRequest(http.MethodGet, "https://management.azure.com/tenants?api-version=2019-11-01", nil)
		if err != nil {
			return err
		}

		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return errors.Wrap(err, "login failed")
		}

		bits, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return errors.Wrap(err, "login failed")
		}

		if res.StatusCode == 200 {
			var tenantResult tenantResult
			if err := json.Unmarshal(bits, &tenantResult); err != nil {
				return errors.Wrap(err, "login failed")
			}
			tenantID := tenantResult.Value[0].TenantID
			tenantToken, err := login.refreshToken(token.RefreshToken, tenantID)
			if err != nil {
				return errors.Wrap(err, "login failed")
			}
			loginInfo := TokenInfo{TenantID: tenantID, Token: tenantToken}

			err = login.tokenStore.writeLoginInfo(loginInfo)

			if err != nil {
				return errors.Wrap(err, "login failed")
			}
			fmt.Println("Login Succeeded")

			return nil
		}

		bits, err = httputil.DumpResponse(res, true)
		if err != nil {
			return errors.Wrap(err, "login failed")
		}

		return fmt.Errorf("login failed: \n" + string(bits))
	}
}

func startLoginServer(queryCh chan url.Values) (int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", queryHandler(queryCh))
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}

	availablePort := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil {
			queryCh <- url.Values{
				"error": []string{fmt.Sprintf("error starting http server with: %v", err)},
			}
		}
	}()
	return availablePort, nil
}

func openAzureLoginPage(redirectURL string) {
	state := randomString("", 10)
	authURL := fmt.Sprintf(authorizeFormat, clientID, redirectURL, state, scopes)
	openbrowser(authURL)
}

func queryHandler(queryCh chan url.Values) func(w http.ResponseWriter, r *http.Request) {
	queryHandler := func(w http.ResponseWriter, r *http.Request) {
		_, hasCode := r.URL.Query()["code"]
		if hasCode {
			_, err := w.Write([]byte(successfullLoginHTML))
			if err != nil {
				queryCh <- url.Values{
					"error": []string{err.Error()},
				}
			} else {
				queryCh <- r.URL.Query()
			}
		} else {
			_, err := w.Write([]byte(loginFailedHTML))
			if err != nil {
				queryCh <- url.Values{
					"error": []string{err.Error()},
				}
			} else {
				queryCh <- r.URL.Query()
			}
		}
	}
	return queryHandler
}

func (helper azureAPIHelper) queryToken(data url.Values, tenantID string) (azureToken, error) {
	res, err := http.Post(fmt.Sprintf(tokenEndpoint, tenantID), "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return azureToken{}, err
	}
	if res.StatusCode != 200 {
		return azureToken{}, errors.Errorf("error while renewing access token, status : %s", res.Status)
	}
	bits, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return azureToken{}, err
	}
	token := azureToken{}
	if err := json.Unmarshal(bits, &token); err != nil {
		return azureToken{}, err
	}
	return token, nil
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

// NewAuthorizerFromLogin creates an authorizer based on login access token
func NewAuthorizerFromLogin() (autorest.Authorizer, error) {
	login, err := NewAzureLoginService()
	if err != nil {
		return nil, err
	}
	oauthToken, err := login.GetValidToken()
	if err != nil {
		return nil, err
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

// GetValidToken returns an access token. Refresh token if needed
func (login AzureLoginService) GetValidToken() (oauth2.Token, error) {
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

func (login AzureLoginService) refreshToken(currentRefreshToken string, tenantID string) (oauth2.Token, error) {
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

func openbrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatal(err)
	}
}

var (
	letterRunes = []rune("abcdefghijklmnopqrstuvwxyz123456789")
)

func randomString(prefix string, length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return prefix + string(b)
}

const loginFailedHTML = `
	<!DOCTYPE html>
	<html>
	<head>
	    <meta charset="utf-8" />
	    <title>Login failed</title>
	</head>
	<body>
	    <h4>Some failures occurred during the authentication</h4>
	    <p>You can log an issue at <a href="https://github.com/azure/azure-cli/issues">Azure CLI GitHub Repository</a> and we will assist you in resolving it.</p>
	</body>
	</html>
	`

const successfullLoginHTML = `
	<!DOCTYPE html>
	<html>
	<head>
	    <meta charset="utf-8" />
	    <meta http-equiv="refresh" content="10;url=https://docs.microsoft.com/cli/azure/">
	    <title>Login successfully</title>
	</head>
	<body>
	    <h4>You have logged into Microsoft Azure!</h4>
	    <p>You can close this window, or we will redirect you to the <a href="https://docs.microsoft.com/cli/azure/">Azure CLI documents</a> in 10 seconds.</p>
	</body>
	</html>
	`
