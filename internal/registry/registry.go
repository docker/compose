/*
   Copyright 2023 Docker Compose CLI authors

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

package registry

import "github.com/distribution/reference"

const (
	// IndexHostname is the index hostname, used for authentication and image search.
	IndexHostname = "index.docker.io"
	// IndexServer is used for user auth and image search
	IndexServer = "https://index.docker.io/v1/"
	// IndexName is the name of the index
	IndexName = "docker.io"
)

// GetAuthConfigKey special-cases using the full index address of the official
// index as the AuthConfig key, and uses the (host)name[:port] for private indexes.
func GetAuthConfigKey(reposName reference.Named) string {
	indexName := reference.Domain(reposName)
	if indexName == IndexName || indexName == IndexHostname {
		return IndexServer
	}
	return indexName
}
