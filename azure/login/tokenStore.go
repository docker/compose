package login

import (
	"encoding/json"
	"io/ioutil"

	"golang.org/x/oauth2"
)

type TokenStore struct {
	filePath string
}

type LoginInfo struct {
	Token    oauth2.Token `json:"oauthToken"`
	TenantID string       `json:"tenantId"`
}

func NewTokenStore(filePath string) TokenStore {
	return TokenStore{
		filePath: filePath,
	}
}

func (store TokenStore) writeLoginInfo(info LoginInfo) error {
	bytes, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	ioutil.WriteFile(store.filePath, bytes, 0644)
	return nil
}

func (store TokenStore) readToken() (loginInfo LoginInfo, err error) {
	bytes, err := ioutil.ReadFile(store.filePath)
	if err != nil {
		return loginInfo, err
	}
	if err := json.Unmarshal(bytes, &loginInfo); err != nil {
		return loginInfo, err
	}
	return loginInfo, nil
}
