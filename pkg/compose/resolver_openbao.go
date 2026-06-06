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
	"fmt"
	"strings"

	openbao "github.com/openbao/openbao/api/v2"
)

// openbaoResolver resolves secrets from OpenBao using KV v2.
// Authentication is handled via environment variables:
//   - BAO_ADDR — server address
//   - BAO_TOKEN — authentication token
//   - BAO_CACERT — CA certificate path
//   - BAO_SKIP_VERIFY — skip TLS verification
type openbaoResolver struct {
	client *openbao.Client
}

func newOpenbaoResolver() (SecretResolver, error) {
	client, err := openbao.NewClient(openbao.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("creating openbao client: %w", err)
	}
	return &openbaoResolver{client: client}, nil
}

// Resolve reads a secret from OpenBao KV v2 at the given path and
// returns the value of the specified key.
func (r *openbaoResolver) Resolve(path, key string) (string, error) {
	kvPath := insertKVv2Data(path)

	secret, err := r.client.Logical().Read(kvPath)
	if err != nil {
		return "", fmt.Errorf("reading %q: %w", path, err)
	}
	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("no data at path %q", path)
	}

	// KV v2 wraps actual data under a "data" key
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected data format at path %q", path)
	}

	val, ok := data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found at path %q", key, path)
	}
	return fmt.Sprintf("%v", val), nil
}

// insertKVv2Data transforms "mount/path/to/secret" into "mount/data/path/to/secret"
// for KV v2 API compatibility. If the second segment is already "data", the path
// is returned unchanged to avoid double insertion.
func insertKVv2Data(path string) string {
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		return path
	}
	if parts[1] == "data" {
		return path
	}
	return parts[0] + "/data/" + strings.Join(parts[1:], "/")
}
