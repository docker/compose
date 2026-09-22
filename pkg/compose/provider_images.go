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
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/pkg/api"
)

const (
	// providerImageSourceLocal: the desired state is the local daemon's
	// image; the provider synchronizes from it (get-image) when its
	// bookkeeping says it holds something else.
	providerImageSourceLocal = "local"
	// providerImageSourceRegistry: the reference is resolvable upstream and
	// upstream is the authority; local facts are an optimization, never an
	// obligation.
	providerImageSourceRegistry = "registry"

	// providerPullPolicyMissing (up path): a usable version present in the
	// provider's runtime suffices.
	providerPullPolicyMissing = "missing"
	// providerPullPolicyAlways (compose pull): the provider must ensure it
	// holds the latest version of the authority.
	providerPullPolicyAlways = "always"
)

// ensureProviderImages gives provider-backed services their turn in the image
// phase: for every service whose provider declares the pull command (metadata
// opt-in, like stop), compose invokes
//
//	<provider> compose pull --image=<ref> [--digest=… --created=…] --source=… --policy=… <service>
//
// The provider owns making the image available to ITS runtime: resolving the
// reference upstream, or requesting the local bytes with a get-image message.
// built maps the image names (re)built by the current run, which forces the
// local-source verdict for them.
func (s *composeService) ensureProviderImages(ctx context.Context, project *types.Project, built map[string]string, policy string) error {
	// deliberately NOT errgroup.WithContext: providers are independent, and
	// fail-fast cancellation would kill the siblings of the first failure
	// mid-run, mangling their reports into context-canceled noise. Every
	// provider runs to completion and every failure is reported.
	var eg errgroup.Group
	eg.SetLimit(s.maxConcurrency)
	var mu sync.Mutex
	var errs []error
	for _, service := range project.Services {
		if service.Provider == nil {
			continue
		}
		image := api.GetImageNameOrDefault(service, project.Name)
		_, justBuilt := built[image]
		source := providerImageSource(service, justBuilt)
		eg.Go(func() error {
			if err := s.runProviderPull(ctx, project, service, image, source, policy); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("provider service %q: %w", service.Name, err))
				mu.Unlock()
			}
			return nil
		})
	}
	_ = eg.Wait()
	return errors.Join(errs...)
}

// providerImageSource computes the authority VERDICT for one invocation —
// compose owns this arbitration (pull_policy, flags, what this run built) so
// providers never re-implement it:
//   - "local": the desired state is the local daemon's image — a declared
//     build with no pullable reference, pull_policy build, or an image the
//     current run just (re)built;
//   - "registry": the provider resolves the reference upstream — including
//     the build-in-CI workflow where a build section is only the recipe used
//     to publish the image consumers pull.
func providerImageSource(service types.ServiceConfig, justBuilt bool) string {
	if service.Build == nil {
		return providerImageSourceRegistry
	}
	if service.Image == "" || justBuilt {
		return providerImageSourceLocal
	}
	if policy, _, err := service.GetPullPolicy(); err == nil && policy == types.PullPolicyBuild {
		return providerImageSourceLocal
	}
	return providerImageSourceRegistry
}

func (s *composeService) runProviderPull(ctx context.Context, project *types.Project, service types.ServiceConfig, image, source, policy string) error {
	args := []string{"--image=" + image, "--source=" + source, "--policy=" + policy}
	if digest, created, ok := s.localImageFacts(ctx, image); ok {
		args = append(args, "--digest="+digest)
		if created != "" {
			args = append(args, "--created="+created)
		}
	}
	return s.runPlugin(ctx, project, service, "pull", args...)
}

// localImageFacts reports the state of the local daemon cache for ref: the
// image ID and its creation time. These describe the cache, they are not
// instructions — the provider persists them as the bookkeeping keys of what
// it ingested (digest as identity test, created as the ordering fallback for
// backends that cannot preserve digests).
func (s *composeService) localImageFacts(ctx context.Context, ref string) (digest, created string, ok bool) {
	inspected, err := s.apiClient().ImageInspect(ctx, ref)
	if err != nil {
		return "", "", false
	}
	return inspected.ID, inspected.Created, true
}
