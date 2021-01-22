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

package store

import "encoding/json"

// DockerContext represents the docker context metadata
type DockerContext struct {
	Name      string                 `json:",omitempty"`
	Metadata  ContextMetadata        `json:",omitempty"`
	Endpoints map[string]interface{} `json:",omitempty"`
}

// Type the context type
func (m *DockerContext) Type() string {
	if m.Metadata.Type == "" {
		return DefaultContextType
	}
	return m.Metadata.Type
}

// ContextMetadata is represtentation of the data we put in a context
// metadata
type ContextMetadata struct {
	Type              string
	Description       string
	StackOrchestrator string
	AdditionalFields  map[string]interface{}
}

// AciContext is the context for the ACI backend
type AciContext struct {
	SubscriptionID string `json:",omitempty"`
	Location       string `json:",omitempty"`
	ResourceGroup  string `json:",omitempty"`
}

// EcsContext is the context for the AWS backend
type EcsContext struct {
	CredentialsFromEnv bool   `json:",omitempty"`
	Profile            string `json:",omitempty"`
}

// KubeContext is the context for a kube backend
type KubeContext struct {
	ContextName     string `json:",omitempty"`
	KubeconfigPath  string `json:",omitempty"`
	FromEnvironment bool
}

// AwsContext is the context for the ecs plugin
type AwsContext EcsContext

// LocalContext is the context for the local backend
type LocalContext struct{}

// MarshalJSON implements custom JSON marshalling
func (dc ContextMetadata) MarshalJSON() ([]byte, error) {
	s := map[string]interface{}{}
	if dc.Description != "" {
		s["Description"] = dc.Description
	}
	if dc.StackOrchestrator != "" {
		s["StackOrchestrator"] = dc.StackOrchestrator
	}
	if dc.Type != "" {
		s["Type"] = dc.Type
	}
	if dc.AdditionalFields != nil {
		for k, v := range dc.AdditionalFields {
			s[k] = v
		}
	}
	return json.Marshal(s)
}

// UnmarshalJSON implements custom JSON marshalling
func (dc *ContextMetadata) UnmarshalJSON(payload []byte) error {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	for k, v := range data {
		switch k {
		case "Description":
			dc.Description = v.(string)
		case "StackOrchestrator":
			dc.StackOrchestrator = v.(string)
		case "Type":
			dc.Type = v.(string)
		default:
			if dc.AdditionalFields == nil {
				dc.AdditionalFields = make(map[string]interface{})
			}
			dc.AdditionalFields[k] = v
		}
	}
	return nil
}
