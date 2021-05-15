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
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure/auth"

	"github.com/pkg/errors"
)

var (
	letterRunes = []rune("abcdefghijklmnopqrstuvwxyz123456789")
)

type apiHelper interface {
	queryToken(ce CloudEnvironment, data url.Values, tenantID string) (azureToken, error)
	openAzureLoginPage(redirectURL string, ce CloudEnvironment) error
	queryAPIWithHeader(ctx context.Context, authorizationURL string, authorizationHeader string) ([]byte, int, error)
	getDeviceCodeFlowToken(ce CloudEnvironment) (adal.Token, error)
}

type azureAPIHelper struct{}

func (helper azureAPIHelper) getDeviceCodeFlowToken(ce CloudEnvironment) (adal.Token, error) {
	deviceconfig := auth.NewDeviceFlowConfig(clientID, "common")
	deviceconfig.Resource = ce.ResourceManagerURL
	spToken, err := deviceconfig.ServicePrincipalToken()
	if err != nil {
		return adal.Token{}, err
	}
	return spToken.Token(), err
}

func (helper azureAPIHelper) openAzureLoginPage(redirectURL string, ce CloudEnvironment) error {
	state := randomString("", 10)
	authURL := fmt.Sprintf(ce.GetAuthorizeRequestFormat(), clientID, redirectURL, state, ce.GetTokenScope())
	return openbrowser(authURL)
}

func (helper azureAPIHelper) queryAPIWithHeader(ctx context.Context, authorizationURL string, authorizationHeader string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, authorizationURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req = req.WithContext(ctx)
	req.Header.Add("Authorization", authorizationHeader)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	bits, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, 0, err
	}
	return bits, res.StatusCode, nil
}

func (helper azureAPIHelper) queryToken(ce CloudEnvironment, data url.Values, tenantID string) (azureToken, error) {
	res, err := http.Post(fmt.Sprintf(ce.GetTokenRequestFormat(), tenantID), "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
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

func openbrowser(address string) error {
	switch runtime.GOOS {
	case "linux":
		if isWsl() {
			return exec.Command("wslview", address).Run()
		}
		return exec.Command("xdg-open", address).Run()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", address).Run()
	case "darwin":
		return exec.Command("open", address).Run()
	default:
		return fmt.Errorf("unsupported platform")
	}
}

func isWsl() bool {
	b, err := ioutil.ReadFile("/proc/version")
	if err != nil {
		return false
	}

	return strings.Contains(strings.ToLower(string(b)), "microsoft")
}

func randomString(prefix string, length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return prefix + string(b)
}
