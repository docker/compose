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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

var (
	letterRunes = []rune("abcdefghijklmnopqrstuvwxyz123456789")
)

type apiHelper interface {
	queryToken(data url.Values, tenantID string) (azureToken, error)
	openAzureLoginPage(redirectURL string) error
	queryAuthorizationAPI(authorizationURL string, authorizationHeader string) ([]byte, int, error)
}

type azureAPIHelper struct{}

func (helper azureAPIHelper) openAzureLoginPage(redirectURL string) error {
	state := randomString("", 10)
	authURL := fmt.Sprintf(authorizeFormat, clientID, redirectURL, state, scopes)
	return openbrowser(authURL)
}

func (helper azureAPIHelper) queryAuthorizationAPI(authorizationURL string, authorizationHeader string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, authorizationURL, nil)
	if err != nil {
		return nil, 0, err
	}
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

func openbrowser(address string) error {
	switch runtime.GOOS {
	case "linux":
		if isWsl() {
			return exec.Command("wslview", address).Start()
		}
		return exec.Command("xdg-open", address).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", address).Start()
	case "darwin":
		return exec.Command("open", address).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}

func isWsl() bool {
	b, err := ioutil.ReadFile("/proc/version")
	if err != nil {
		return false
	}

	return strings.Contains(string(b), "microsoft")
}

func randomString(prefix string, length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return prefix + string(b)
}
