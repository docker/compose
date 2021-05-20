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

package convert

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/oauth2"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerinstance/mgmt/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	compose "github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/aci/login"
)

const (
	// Specific username from ACR docs : https://github.com/Azure/acr/blob/master/docs/AAD-OAuth.md#getting-credentials-programatically
	tokenUsername = "00000000-0000-0000-0000-000000000000"
	dockerHub     = "index.docker.io"
)

type registryHelper interface {
	getAllRegistryCredentials() (map[string]types.AuthConfig, error)
	autoLoginAcr(registry string, loginService login.AzureLoginService) error
}

type cliRegistryHelper struct {
	cfg *configfile.ConfigFile
}

func (c cliRegistryHelper) getAllRegistryCredentials() (map[string]types.AuthConfig, error) {
	return c.cfg.GetAllCredentials()
}

func newCliRegistryConfLoader() cliRegistryHelper {
	return cliRegistryHelper{
		cfg: config.LoadDefaultConfigFile(os.Stderr),
	}
}

func getRegistryCredentials(project compose.Project, helper registryHelper) ([]containerinstance.ImageRegistryCredential, error) {
	loginService, err := login.NewAzureLoginService()
	if err != nil {
		return nil, err
	}

	var cloudEnvironment *login.CloudEnvironment
	if ce, err := loginService.GetCloudEnvironment(); err != nil {
		cloudEnvironment = &ce
	}

	usedRegistries, acrRegistries := getUsedRegistries(project, cloudEnvironment)
	for _, registry := range acrRegistries {
		err := helper.autoLoginAcr(registry, loginService)
		if err != nil {
			fmt.Printf("WARNING: %v\n", err)
			fmt.Printf("Could not automatically login to %s from your Azure login. Assuming you already logged in to the ACR registry\n", registry)
		}
	}

	allCreds, err := helper.getAllRegistryCredentials()
	if err != nil {
		return nil, err
	}
	var registryCreds []containerinstance.ImageRegistryCredential
	for name, oneCred := range allCreds {
		parsedURL, err := url.Parse(name)
		// Credentials can contain some garbage, we don't return the error here
		// because we don't care about these garbage creds.
		if err != nil {
			continue
		}

		hostname := parsedURL.Host
		if hostname == "" {
			hostname = parsedURL.Path
		}
		if _, ok := usedRegistries[hostname]; ok {
			if oneCred.Password != "" {
				aciCredential := containerinstance.ImageRegistryCredential{
					Server:   to.StringPtr(hostname),
					Password: to.StringPtr(oneCred.Password),
					Username: to.StringPtr(oneCred.Username),
				}
				registryCreds = append(registryCreds, aciCredential)
			} else if oneCred.IdentityToken != "" {
				userName := tokenUsername
				if oneCred.Username != "" {
					userName = oneCred.Username
				}
				aciCredential := containerinstance.ImageRegistryCredential{
					Server:   to.StringPtr(hostname),
					Password: to.StringPtr(oneCred.IdentityToken),
					Username: to.StringPtr(userName),
				}
				registryCreds = append(registryCreds, aciCredential)
			}
		}
	}
	return registryCreds, nil
}

func getUsedRegistries(project compose.Project, ce *login.CloudEnvironment) (map[string]bool, []string) {
	usedRegistries := map[string]bool{}
	acrRegistries := []string{}

	for _, service := range project.Services {
		imageName := service.Image
		tokens := strings.Split(imageName, "/")
		registry := tokens[0]
		if len(tokens) == 1 { // ! image names can include "." ...
			registry = dockerHub
		} else if !strings.Contains(registry, ".") {
			registry = dockerHub
		} else if ce != nil {
			if suffix, present := ce.Suffixes[login.AcrSuffixKey]; present && strings.HasSuffix(registry, suffix) {
				acrRegistries = append(acrRegistries, registry)
			}
		}
		usedRegistries[registry] = true
	}
	return usedRegistries, acrRegistries
}

func (c cliRegistryHelper) autoLoginAcr(registry string, loginService login.AzureLoginService) error {
	token, tenantID, err := loginService.GetValidToken()
	if err != nil {
		return err
	}

	data := url.Values{
		"grant_type":   {"access_token"},
		"service":      {registry},
		"tenant":       {tenantID},
		"access_token": {token.AccessToken},
	}
	repoAuthURL := fmt.Sprintf("https://%s/oauth2/exchange", registry)
	res, err := http.Post(repoAuthURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return errors.Wrap(err, "could not query ACR token")
	}
	bits, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return errors.Wrap(err, "could not read response body")
	}
	if res.StatusCode != 200 {
		return errors.Errorf("could not obtain ACR token from Azure login, status : %s, response: %s", res.Status, string(bits))
	}

	newToken := oauth2.Token{}
	if err := json.Unmarshal(bits, &newToken); err != nil {
		return errors.Wrap(err, "could not read ACR token")
	}
	cmd := exec.Command("docker", "login", "-u", tokenUsername, "-p", newToken.RefreshToken, registry)
	bytes, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("could not 'docker login' to %s :\n%s\n", registry, string(bytes)))
	}
	return nil
}
