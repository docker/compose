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
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Azure/go-autorest/autorest/azure/cli"

	"golang.org/x/oauth2"
)

type tokenStore struct {
	filePath string
}

// TokenInfo data stored in tokenStore
type TokenInfo struct {
	Token            oauth2.Token `json:"oauthToken"`
	TenantID         string       `json:"tenantId"`
	CloudEnvironment string       `json:"cloudEnvironment"`
}

func newTokenStore(path string) (tokenStore, error) {
	parentFolder := filepath.Dir(path)
	dir, err := os.Stat(parentFolder)
	if os.IsNotExist(err) {
		err = os.MkdirAll(parentFolder, 0700)
		if err != nil {
			return tokenStore{}, err
		}
		dir, err = os.Stat(parentFolder)
	}
	if err != nil {
		return tokenStore{}, err
	}
	if !dir.Mode().IsDir() {
		return tokenStore{}, errors.New("cannot use path " + path + " ; " + parentFolder + " already exists and is not a directory")
	}
	return tokenStore{
		filePath: path,
	}, nil
}

// GetTokenStorePath the path for token store
func GetTokenStorePath() string {
	cliPath, _ := cli.AccessTokensPath()
	return filepath.Join(filepath.Dir(cliPath), tokenStoreFilename)
}

func (store tokenStore) writeLoginInfo(info TokenInfo) error {
	bytes, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(store.filePath, bytes, 0644)
}

func (store tokenStore) readToken() (TokenInfo, error) {
	bytes, err := ioutil.ReadFile(store.filePath)
	if err != nil {
		return TokenInfo{}, err
	}
	loginInfo := TokenInfo{}
	if err := json.Unmarshal(bytes, &loginInfo); err != nil {
		return TokenInfo{}, err
	}
	if loginInfo.CloudEnvironment == "" {
		loginInfo.CloudEnvironment = AzurePublicCloudName
	}
	return loginInfo, nil
}

func (store tokenStore) removeData() error {
	return os.Remove(store.filePath)
}
