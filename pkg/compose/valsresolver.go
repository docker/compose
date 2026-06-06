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

	"github.com/compose-spec/compose-go/v2/types"
)

const refPrefix = "ref+"

// SecretResolver resolves a secret reference to its actual value.
// Implementations handle backend-specific logic (API calls, auth, etc.).
type SecretResolver interface {
	Resolve(path, key string) (string, error)
}

// resolverFactory creates a resolver instance. Called lazily on first use.
type resolverFactory func() (SecretResolver, error)

// resolverRegistry maps URI scheme prefixes to their factory functions.
// To add a new backend, register it here and implement SecretResolver.
var resolverRegistry = map[string]resolverFactory{
	"ref+openbao://": newOpenbaoResolver,
}

// resolveSecretReferences resolves environment values prefixed with "ref+"
// by dispatching to the appropriate backend resolver based on URI scheme.
func resolveSecretReferences(project *types.Project) error {
	if !projectHasRefs(project) {
		return nil
	}

	// Cache resolver instances so we only create one per backend
	resolvers := map[string]SecretResolver{}

	for name, svc := range project.Services {
		for k, v := range svc.Environment {
			if v == nil || !strings.HasPrefix(*v, refPrefix) {
				continue
			}

			resolver, err := getResolver(*v, resolvers)
			if err != nil {
				return fmt.Errorf("resolving %q for service %q: %w", k, name, err)
			}

			path, key, err := parseRef(*v)
			if err != nil {
				return fmt.Errorf("resolving %q for service %q: %w", k, name, err)
			}

			resolved, err := resolver.Resolve(path, key)
			if err != nil {
				return fmt.Errorf("resolving %q for service %q: %w", k, name, err)
			}
			svc.Environment[k] = &resolved
		}
		project.Services[name] = svc
	}
	return nil
}

// getResolver returns the cached resolver for the given ref URI, creating it
// on first use via the registry factory.
func getResolver(ref string, cache map[string]SecretResolver) (SecretResolver, error) {
	for prefix, factory := range resolverRegistry {
		if strings.HasPrefix(ref, prefix) {
			if r, ok := cache[prefix]; ok {
				return r, nil
			}
			r, err := factory()
			if err != nil {
				return nil, err
			}
			cache[prefix] = r
			return r, nil
		}
	}
	return nil, fmt.Errorf("unsupported secret reference scheme in %q", ref)
}

// parseRef extracts the path and key from a ref+ URI.
// Format: ref+<backend>://path/to/secret#/key
func parseRef(ref string) (string, string, error) {
	// Strip the "ref+<scheme>://" prefix
	for prefix := range resolverRegistry {
		if strings.HasPrefix(ref, prefix) {
			ref = strings.TrimPrefix(ref, prefix)
			break
		}
	}

	parts := strings.SplitN(ref, "#", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid ref %q: missing #/key", ref)
	}

	path := parts[0]
	key := strings.TrimPrefix(parts[1], "/")
	return path, key, nil
}

// projectHasRefs returns true if any service environment value starts with "ref+",
// allowing early exit when no resolution is needed.
func projectHasRefs(project *types.Project) bool {
	for _, svc := range project.Services {
		for _, v := range svc.Environment {
			if v != nil && strings.HasPrefix(*v, refPrefix) {
				return true
			}
		}
	}
	return false
}
