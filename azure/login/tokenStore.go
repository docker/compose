package login

import (
	"encoding/json"
	"io/ioutil"

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

func (store tokenStore) writeLoginInfo(info TokenInfo) error {
	bytes, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(store.filePath, bytes, 0644)
}

func (store tokenStore) readToken() (loginInfo TokenInfo, err error) {
	bytes, err := ioutil.ReadFile(store.filePath)
	if err != nil {
		return loginInfo, err
	}
	if err := json.Unmarshal(bytes, &loginInfo); err != nil {
		return loginInfo, err
	}
	return loginInfo, nil
}
