package login

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
)

type tokenStore struct {
	filePath string
}

// TokenInfo data stored in tokenStore
type TokenInfo struct {
	Token    oauth2.Token `json:"oauthToken"`
	TenantID string       `json:"tenantId"`
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
	return loginInfo, nil
}
