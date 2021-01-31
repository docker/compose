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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/pkg/errors"
)

const (
	// AzurePublicCloudName is the moniker of the Azure public cloud
	AzurePublicCloudName = "AzureCloud"

	// AcrSuffixKey is the well-known name of the DNS suffix for Azure Container Registries
	AcrSuffixKey = "acrLoginServer"

	// CloudMetadataURLVar is the name of the environment variable that (if defined), points to a URL that should be used by Docker CLI to retrieve cloud metadata
	CloudMetadataURLVar = "ARM_CLOUD_METADATA_URL"

	// DefaultCloudMetadataURL is the URL of the cloud metadata service maintained by Azure public cloud
	DefaultCloudMetadataURL = "https://management.azure.com/metadata/endpoints?api-version=2019-05-01"
)

// CloudEnvironmentService exposed metadata about Azure cloud environments
type CloudEnvironmentService interface {
	Get(name string) (CloudEnvironment, error)
}

type cloudEnvironmentService struct {
	cloudEnvironments map[string]CloudEnvironment
	cloudMetadataURL  string
	// True if we have queried the cloud metadata endpoint already.
	// We do it only once per CLI invocation.
	metadataQueried bool
}

var (
	// CloudEnvironments is the default instance of the CloudEnvironmentService
	CloudEnvironments CloudEnvironmentService
)

func init() {
	CloudEnvironments = newCloudEnvironmentService()
}

// CloudEnvironmentAuthentication data for logging into, and obtaining tokens for, Azure sovereign clouds
type CloudEnvironmentAuthentication struct {
	LoginEndpoint string   `json:"loginEndpoint"`
	Audiences     []string `json:"audiences"`
	Tenant        string   `json:"tenant"`
}

// CloudEnvironment describes Azure sovereign cloud instance (e.g. Azure public, Azure US government, Azure China etc.)
type CloudEnvironment struct {
	Name               string                         `json:"name"`
	Authentication     CloudEnvironmentAuthentication `json:"authentication"`
	ResourceManagerURL string                         `json:"resourceManager"`
	Suffixes           map[string]string              `json:"suffixes"`
}

func newCloudEnvironmentService() *cloudEnvironmentService {
	retval := cloudEnvironmentService{
		metadataQueried: false,
	}
	retval.resetCloudMetadata()
	return &retval
}

func (ces *cloudEnvironmentService) Get(name string) (CloudEnvironment, error) {
	if ce, present := ces.cloudEnvironments[name]; present {
		return ce, nil
	}

	if !ces.metadataQueried {
		ces.metadataQueried = true

		if ces.cloudMetadataURL == "" {
			ces.cloudMetadataURL = os.Getenv(CloudMetadataURLVar)
			if _, err := url.ParseRequestURI(ces.cloudMetadataURL); err != nil {
				ces.cloudMetadataURL = DefaultCloudMetadataURL
			}
		}

		res, err := http.Get(ces.cloudMetadataURL)
		if err != nil {
			return CloudEnvironment{}, fmt.Errorf("Cloud metadata retrieval from '%s' failed: %w", ces.cloudMetadataURL, err)
		}
		if res.StatusCode != 200 {
			return CloudEnvironment{}, errors.Errorf("Cloud metadata retrieval from '%s' failed: server response was '%s'", ces.cloudMetadataURL, res.Status)
		}

		bytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return CloudEnvironment{}, fmt.Errorf("Cloud metadata retrieval from '%s' failed: %w", ces.cloudMetadataURL, err)
		}

		if err = ces.applyCloudMetadata(bytes); err != nil {
			return CloudEnvironment{}, fmt.Errorf("Cloud metadata retrieval from '%s' failed: %w", ces.cloudMetadataURL, err)
		}
	}

	if ce, present := ces.cloudEnvironments[name]; present {
		return ce, nil
	}

	return CloudEnvironment{}, errors.Errorf("Cloud environment '%s' was not found", name)
}

func (ces *cloudEnvironmentService) applyCloudMetadata(jsonBytes []byte) error {
	input := []CloudEnvironment{}
	if err := json.Unmarshal(jsonBytes, &input); err != nil {
		return err
	}

	newEnvironments := make(map[string]CloudEnvironment, len(input))
	// If _any_ of the submitted data is invalid, we bail out.
	for _, e := range input {
		if len(e.Name) == 0 {
			return errors.New("Azure cloud environment metadata is invalid (an environment with no name has been encountered)")
		}

		e.normalizeURLs()

		if _, err := url.ParseRequestURI(e.Authentication.LoginEndpoint); err != nil {
			return errors.Errorf("Metadata of cloud environment '%s' has invalid login endpoint URL: %s", e.Name, e.Authentication.LoginEndpoint)
		}

		if _, err := url.ParseRequestURI(e.ResourceManagerURL); err != nil {
			return errors.Errorf("Metadata of cloud environment '%s' has invalid resource manager URL: %s", e.Name, e.ResourceManagerURL)
		}

		if len(e.Authentication.Audiences) == 0 {
			return errors.Errorf("Metadata of cloud environment '%s' is invalid (no authentication audiences)", e.Name)
		}

		newEnvironments[e.Name] = e
	}

	for name, e := range newEnvironments {
		ces.cloudEnvironments[name] = e
	}
	return nil
}

func (ces *cloudEnvironmentService) resetCloudMetadata() {
	azurePublicCloud := CloudEnvironment{
		Name: AzurePublicCloudName,
		Authentication: CloudEnvironmentAuthentication{
			LoginEndpoint: "https://login.microsoftonline.com",
			Audiences: []string{
				"https://management.core.windows.net",
				"https://management.azure.com",
			},
			Tenant: "common",
		},
		ResourceManagerURL: "https://management.azure.com",
		Suffixes: map[string]string{
			AcrSuffixKey: "azurecr.io",
		},
	}

	azureChinaCloud := CloudEnvironment{
		Name: "AzureChinaCloud",
		Authentication: CloudEnvironmentAuthentication{
			LoginEndpoint: "https://login.chinacloudapi.cn",
			Audiences: []string{
				"https://management.core.chinacloudapi.cn",
				"https://management.chinacloudapi.cn",
			},
			Tenant: "common",
		},
		ResourceManagerURL: "https://management.chinacloudapi.cn",
		Suffixes: map[string]string{
			AcrSuffixKey: "azurecr.cn",
		},
	}

	azureUSGovernment := CloudEnvironment{
		Name: "AzureUSGovernment",
		Authentication: CloudEnvironmentAuthentication{
			LoginEndpoint: "https://login.microsoftonline.us",
			Audiences: []string{
				"https://management.core.usgovcloudapi.net",
				"https://management.usgovcloudapi.net",
			},
			Tenant: "common",
		},
		ResourceManagerURL: "https://management.usgovcloudapi.net",
		Suffixes: map[string]string{
			AcrSuffixKey: "azurecr.us",
		},
	}

	azureGermanCloud := CloudEnvironment{
		Name: "AzureGermanCloud",
		Authentication: CloudEnvironmentAuthentication{
			LoginEndpoint: "https://login.microsoftonline.de",
			Audiences: []string{
				"https://management.core.cloudapi.de",
				"https://management.microsoftazure.de",
			},
			Tenant: "common",
		},
		ResourceManagerURL: "https://management.microsoftazure.de",

		// There is no separate container registry suffix for German cloud
		Suffixes: map[string]string{},
	}

	ces.cloudEnvironments = map[string]CloudEnvironment{
		azurePublicCloud.Name:  azurePublicCloud,
		azureChinaCloud.Name:   azureChinaCloud,
		azureUSGovernment.Name: azureUSGovernment,
		azureGermanCloud.Name:  azureGermanCloud,
	}
}

// GetTenantQueryURL returns an URL that can be used to fetch the list of Azure Active Directory tenants from a given cloud environment
func (ce *CloudEnvironment) GetTenantQueryURL() string {
	tenantURL := ce.ResourceManagerURL + "/tenants?api-version=2019-11-01"
	return tenantURL
}

// GetTokenScope returns a token scope that fits Docker CLI Azure management API usage
func (ce *CloudEnvironment) GetTokenScope() string {
	scope := "offline_access " + ce.ResourceManagerURL + "/.default"
	return scope
}

// GetAuthorizeRequestFormat returns a string format that can be used to construct authorization code request in an OAuth2 flow.
// The URL uses login endpoint appropriate for given cloud environment.
func (ce *CloudEnvironment) GetAuthorizeRequestFormat() string {
	authorizeFormat := ce.Authentication.LoginEndpoint + "/organizations/oauth2/v2.0/authorize?response_type=code&client_id=%s&redirect_uri=%s&state=%s&prompt=select_account&response_mode=query&scope=%s"
	return authorizeFormat
}

// GetTokenRequestFormat returns a string format that can be used to construct a security token request against Azure Active Directory
func (ce *CloudEnvironment) GetTokenRequestFormat() string {
	tokenEndpoint := ce.Authentication.LoginEndpoint + "/%s/oauth2/v2.0/token"
	return tokenEndpoint
}

func (ce *CloudEnvironment) normalizeURLs() {
	ce.ResourceManagerURL = removeTrailingSlash(ce.ResourceManagerURL)
	ce.Authentication.LoginEndpoint = removeTrailingSlash(ce.Authentication.LoginEndpoint)
	for i, s := range ce.Authentication.Audiences {
		ce.Authentication.Audiences[i] = removeTrailingSlash(s)
	}
}

func removeTrailingSlash(s string) string {
	return strings.TrimSuffix(s, "/")
}
