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

package compose

import (
	"encoding/json"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/opencontainers/go-digest"
)

// ServiceHash computes the configuration hash for a service.
func ServiceHash(o types.ServiceConfig) (string, error) {
	// remove the Build config when generating the service hash
	o.Build = nil
	o.PullPolicy = ""
	o.Scale = nil
	if o.Deploy != nil {
		o.Deploy.Replicas = nil
	}
	o.DependsOn = nil
	o.Profiles = nil

	bytes, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	return digest.SHA256.FromBytes(bytes).Encoded(), nil
}
