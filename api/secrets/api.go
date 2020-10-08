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
	"context"
	"encoding/json"
)

// Service interacts with the underlying secrets backend
type Service interface {
	CreateSecret(ctx context.Context, secret Secret) (string, error)
	InspectSecret(ctx context.Context, id string) (Secret, error)
	ListSecrets(ctx context.Context) ([]Secret, error)
	DeleteSecret(ctx context.Context, id string, recover bool) error
}

// Secret hold sensitive data
type Secret struct {
	ID      string            `json:"ID"`
	Name    string            `json:"Name"`
	Labels  map[string]string `json:"Tags"`
	content []byte
}

// NewSecret builds a secret
func NewSecret(name string, content []byte) Secret {
	return Secret{
		Name:    name,
		content: content,
	}
}

// ToJSON marshall a Secret into JSON string
func (s Secret) ToJSON() (string, error) {
	b, err := json.MarshalIndent(&s, "", "\t")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// GetContent returns a Secret's sensitive data
func (s Secret) GetContent() []byte {
	return s.content
}
