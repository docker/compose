package login

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/docker/api/context/store"
)

const authenticationURL = "https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s/listKeys?api-version=2019-06-01"

// StorageAccountHelper helper for Azure Storage Account
type StorageAccountHelper struct {
	LoginService AzureLoginService
	AciContext   store.AciContext
}

type storageAcountKeys struct {
	Keys []storageAcountKey `json:"keys"`
}
type storageAcountKey struct {
	KeyName string `json:"keyName"`
	Value   string `json:"value"`
}

// GetAzureStorageAccountKey retrieves the storage account ket from the current azure login
func (helper StorageAccountHelper) GetAzureStorageAccountKey(accountName string) (string, error) {
	token, err := helper.LoginService.GetValidToken()
	if err != nil {
		return "", err
	}
	authURL := fmt.Sprintf(authenticationURL, helper.AciContext.SubscriptionID, helper.AciContext.ResourceGroup, accountName)
	req, err := http.NewRequest(http.MethodPost, authURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	bits, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("could not access storage account acountKeys for %s, using the azure login. Status %d : %s", accountName, res.StatusCode, string(bits))
	}

	acountKeys := storageAcountKeys{}
	if err := json.Unmarshal(bits, &acountKeys); err != nil {
		return "", err
	}
	if len(acountKeys.Keys) < 1 {
		return "", fmt.Errorf("no key could be obtained for storage account %s from your azure login", accountName)
	}
	return acountKeys.Keys[0].Value, nil
}
