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

package secrets

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// Secret define sensitive data to be bound as file
type Secret struct {
	Name string
	Keys []string
}

// CreateSecretFiles retrieve sensitive data from env and store as plain text a a file in path
func CreateSecretFiles(secret Secret, path string) error {
	value, ok := os.LookupEnv(secret.Name)
	if !ok {
		return fmt.Errorf("%q variable not set", secret.Name)
	}

	secrets := filepath.Join(path, secret.Name)

	if len(secret.Keys) == 0 {
		// raw Secret
		fmt.Printf("inject Secret %q info %s\n", secret.Name, secrets)
		return ioutil.WriteFile(secrets, []byte(value), 0444)
	}

	var unmarshalled interface{}
	err := json.Unmarshal([]byte(value), &unmarshalled)
	if err != nil {
		return fmt.Errorf("%q Secret is not a valid JSON document: %w", secret.Name, err)
	}

	dict, ok := unmarshalled.(map[string]interface{})
	if !ok {
		return fmt.Errorf("%q Secret is not a JSON dictionary: %w", secret.Name, err)
	}
	err = os.MkdirAll(secrets, 0755)
	if err != nil {
		return err
	}

	if contains(secret.Keys, "*") {
		var keys []string
		for k := range dict {
			keys = append(keys, k)
		}
		secret.Keys = keys
	}

	for _, k := range secret.Keys {
		path := filepath.Join(secrets, k)
		fmt.Printf("inject Secret %q info %s\n", k, path)

		v, ok := dict[k]
		if !ok {
			return fmt.Errorf("%q Secret has no %q key", secret.Name, k)
		}

		var raw []byte
		if s, ok := v.(string); ok {
			raw = []byte(s)
		} else {
			raw, err = json.Marshal(v)
			if err != nil {
				return err
			}
		}

		err = ioutil.WriteFile(path, raw, 0444)
		if err != nil {
			return err
		}
	}
	return nil
}

func contains(keys []string, s string) bool {
	for _, k := range keys {
		if k == s {
			return true
		}
	}
	return false
}
