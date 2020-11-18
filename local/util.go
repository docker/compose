// +build local

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

package local

import (
	"encoding/json"

	"github.com/opencontainers/go-digest"
)

func jsonHash(o interface{}) (string, error) {
	bytes, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	return digest.SHA256.FromBytes(bytes).String(), nil
}

func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func containsAll(slice []string, items []string) bool {
	for _, i := range items {
		if !contains(slice, i) {
			return false
		}
	}
	return true
}
