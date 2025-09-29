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

package oci

import (
	"context"
	"io"

	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/compose/v2/internal/registry"
	spec "github.com/opencontainers/image-spec/specs-go/v1"
)

// NewResolver setup an OCI Resolver based on docker/cli config to provide registry credentials
func NewResolver(config *configfile.ConfigFile) remotes.Resolver {
	return docker.NewResolver(docker.ResolverOptions{
		Hosts: docker.ConfigureDefaultRegistries(
			docker.WithAuthorizer(docker.NewDockerAuthorizer(
				docker.WithAuthCreds(func(host string) (string, string, error) {
					host = registry.GetAuthConfigKey(host)
					auth, err := config.GetAuthConfig(host)
					if err != nil {
						return "", "", err
					}
					if auth.IdentityToken != "" {
						return "", auth.IdentityToken, nil
					}
					return auth.Username, auth.Password, nil
				}),
			)),
		),
	})
}

// Get retrieves a Named OCI resource and returns OCI Descriptor and Manifest
func Get(ctx context.Context, resolver remotes.Resolver, ref reference.Named) (spec.Descriptor, []byte, error) {
	_, descriptor, err := resolver.Resolve(ctx, ref.String())
	if err != nil {
		return spec.Descriptor{}, nil, err
	}

	fetcher, err := resolver.Fetcher(ctx, ref.String())
	if err != nil {
		return spec.Descriptor{}, nil, err
	}
	fetch, err := fetcher.Fetch(ctx, descriptor)
	if err != nil {
		return spec.Descriptor{}, nil, err
	}
	content, err := io.ReadAll(fetch)
	if err != nil {
		return spec.Descriptor{}, nil, err
	}
	return descriptor, content, nil
}
