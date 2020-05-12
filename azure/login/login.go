package login

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure/cli"
	"github.com/Azure/go-autorest/autorest/date"
	"golang.org/x/oauth2"

	"github.com/pkg/errors"
)

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
	Token struct {
		Type         string `json:"token_type"`
		Scope        string `json:"scope"`
		ExpiresIn    int    `json:"expires_in"`
		ExtExpiresIn int    `json:"ext_expires_in"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Foci         string `json:"foci"`
	}

	TenantResult struct {
		Value []TenantValue `json:"value"`
	}
	TenantValue struct {
		TenantID string `json:"tenantId"`
	}
)

//AzureLogin login through browser
func Login() error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	queryCh := make(chan url.Values, 1)
	queryHandler := func(w http.ResponseWriter, r *http.Request) {
		queryCh <- r.URL.Query()
		_, hasCode := r.URL.Query()["code"]
		if hasCode {
			w.Write([]byte(`
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
		`))
		} else {
			w.Write([]byte(`
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
`))
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", queryHandler)
	server := &http.Server{Addr: ":8401", Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			fmt.Println(fmt.Errorf("error starting http server with: %w", err))
			os.Exit(1)
		}
	}()

	state := RandomString("", 10)
	//nonce := RandomString("", 10)
	authURL := fmt.Sprintf(authorizeFormat, clientID, "http://localhost:8401", state, scopes)
	openbrowser(authURL)

	select {
	case <-sigs:
		return nil
	case qsValues := <-queryCh:
		code, hasCode := qsValues["code"]
		if !hasCode {
			return fmt.Errorf("Authentication Error : Login failed")
		}
		data := url.Values{
			"grant_type":   []string{"authorization_code"},
			"client_id":    []string{clientID},
			"code":         code,
			"scope":        []string{scopes},
			"redirect_uri": []string{"http://localhost:8401"},
		}
		token, err := queryToken(data, "organizations")
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
			return errors.Wrap(err, "Authentication Error")
		}

		bits, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return errors.Wrap(err, "Authentication Error")
		}

		if res.StatusCode == 200 {
			var tenantResult TenantResult
			if err := json.Unmarshal(bits, &tenantResult); err != nil {
				return errors.Wrap(err, "Authentication Error")
			}
			tenantID := tenantResult.Value[0].TenantID
			tenantToken, err := refreshToken(token.RefreshToken, tenantID)
			if err != nil {
				return errors.Wrap(err, "Authentication Error")
			}
			loginInfo := LoginInfo{TenantID: tenantID, Token: tenantToken}

			store := NewTokenStore(getTokenPath())
			err = store.writeLoginInfo(loginInfo)

			if err != nil {
				return errors.Wrap(err, "Authentication Error")
			}
			fmt.Println("Successfully logged in")

			return nil
		}

		bits, err = httputil.DumpResponse(res, true)
		if err != nil {
			return errors.Wrap(err, "Authentication Error")
		}

		return fmt.Errorf("Authentication Error: \n" + string(bits))
	}
}

func queryToken(data url.Values, tenantID string) (token Token, err error) {
	res, err := http.Post(fmt.Sprintf(tokenEndpoint, tenantID), "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return token, err
	}
	if res.StatusCode != 200 {
		return token, err
	}
	bits, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return token, err
	}
	if err := json.Unmarshal(bits, &token); err != nil {
		return token, err
	}
	return token, nil
}

func toOAuthToken(token Token) oauth2.Token {
	expireTime := time.Now().Add(time.Duration(token.ExtExpiresIn) * time.Second)
	oauthToken := oauth2.Token{
		RefreshToken: token.RefreshToken,
		AccessToken:  token.AccessToken,
		Expiry:       expireTime,
		TokenType:    token.Type,
	}
	return oauthToken
}

const tokenFilename = "dockerAccessToken.json"

func getTokenPath() string {
	cliPath, _ := cli.AccessTokensPath()

	return filepath.Join(filepath.Dir(cliPath), tokenFilename)
}

func NewAuthorizerFromLogin() (autorest.Authorizer, error) {
	oauthToken, err := GetValidToken()
	if err != nil {
		return nil, err
	}

	difference := oauthToken.Expiry.Sub(date.UnixEpoch())

	token := adal.Token{
		AccessToken:  oauthToken.AccessToken,
		Type:         oauthToken.TokenType,
		ExpiresIn:    "3600",
		ExpiresOn:    json.Number(strconv.Itoa(int(difference.Seconds()))),
		RefreshToken: "",
		Resource:     "",
	}

	return autorest.NewBearerAuthorizer(&token), nil
}

func GetValidToken() (token oauth2.Token, err error) {
	store := NewTokenStore(getTokenPath())
	loginInfo, err := store.readToken()
	if err != nil {
		return token, err
	}
	token = loginInfo.Token
	if token.Valid() {
		return token, nil
	}
	tenantID := loginInfo.TenantID
	token, err = refreshToken(token.RefreshToken, tenantID)
	if err != nil {
		return token, errors.Wrap(err, "Access token request failed. Maybe you need to login to azure again.")
	}
	err = store.writeLoginInfo(LoginInfo{TenantID: tenantID, Token: token})
	if err != nil {
		return token, err
	}
	return token, nil
}

func refreshToken(currentRefreshToken string, tenantID string) (oauthToken oauth2.Token, err error) {
	data := url.Values{
		"grant_type":    []string{"refresh_token"},
		"client_id":     []string{clientID},
		"scope":         []string{scopes},
		"refresh_token": []string{currentRefreshToken},
	}
	token, err := queryToken(data, tenantID)
	if err != nil {
		return oauthToken, err
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

func init() {
	rand.Seed(time.Now().Unix())
}

// RandomString generates a random string with prefix
func RandomString(prefix string, length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return prefix + string(b)
}
