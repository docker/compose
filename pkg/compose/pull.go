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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/platforms"
	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config/configfile"
	clitypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/go-units"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/internal/registry"
	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Pull(ctx context.Context, project *types.Project, options api.PullOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.pull(ctx, project, options)
	}, "pull", s.events)
}

// imagePuller tracks the state of a `compose pull` run: the images already
// scheduled (a same image may back several services), per-service pull
// failures, and services whose image must be built as a fallback.
type imagePuller struct {
	*composeService
	project    *types.Project
	opts       api.PullOptions
	images     map[string]api.ImageSummary
	eg         *errgroup.Group
	scheduled  map[string]string // image -> first service pulling it
	pullErrors []error
	mu         sync.Mutex
	mustBuild  []string
}

func (s *composeService) pull(ctx context.Context, project *types.Project, opts api.PullOptions) error {
	images, _, err := s.getLocalImagesDigests(ctx, project)
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(s.maxConcurrency)

	p := &imagePuller{
		composeService: s,
		project:        project,
		opts:           opts,
		images:         images,
		eg:             eg,
		scheduled:      map[string]string{},
		pullErrors:     make([]error, len(project.Services)),
	}

	err = p.pullServiceImages(ctx)
	if err == nil {
		err = p.pullHookImages(ctx)
	}
	if err != nil {
		// join already-scheduled pulls before returning: bailing out with
		// goroutines still in flight would leak them past pull()'s return
		return errors.Join(err, eg.Wait())
	}

	err = eg.Wait()

	if len(p.mustBuild) > 0 {
		logrus.Warnf("WARNING: Some service image(s) must be built from source by running:\n    docker compose build %s", strings.Join(p.mustBuild, " "))
	}

	if err != nil {
		return err
	}
	if opts.IgnoreFailures {
		return nil
	}
	return errors.Join(p.pullErrors...)
}

// pullServiceImages schedules a pull for each service image which requires
// one, emitting a Skipped event for the others
func (p *imagePuller) pullServiceImages(ctx context.Context) error {
	i := 0
	for name, service := range p.project.Services {
		if service.Image == "" {
			p.eventSkippedPull(name, "No image to be pulled")
			continue
		}

		pullRequired, skipReason, err := shouldPullImage(service, p.images)
		if err != nil {
			return err
		}
		if !pullRequired {
			p.eventSkippedPull("Image "+service.Image, skipReason)
			continue
		}
		if service.Build != nil && p.opts.IgnoreBuildable {
			p.eventSkippedPull("Image "+service.Image, "Image can be built")
			continue
		}
		if _, ok := p.scheduled[service.Image]; ok {
			continue
		}
		p.scheduled[service.Image] = service.Name

		idx := i
		p.eg.Go(func() error {
			return p.runServicePull(ctx, idx, service)
		})
		i++
	}
	return nil
}

// runServicePull pulls a service image, recording the failure and whether the
// image could be built instead, per the fail-fast rules of `compose pull`
func (p *imagePuller) runServicePull(ctx context.Context, idx int, service types.ServiceConfig) error {
	err := p.pullServiceImage(ctx, service, p.opts.Quiet, p.project.Environment["DOCKER_DEFAULT_PLATFORM"])
	if err == nil {
		return nil
	}
	p.pullErrors[idx] = err
	if service.Build != nil {
		p.mu.Lock()
		p.mustBuild = append(p.mustBuild, service.Name)
		p.mu.Unlock()
	}
	if !p.opts.IgnoreFailures && service.Build == nil {
		if p.dryRun {
			p.events.On(errorEventf("Image "+service.Image,
				"error pulling image: %s", service.Image))
		}
		// fail fast if image can't be pulled nor built
		return err
	}
	return nil
}

// pullHookImages schedules pulls for pre_start hook images, which run as
// ephemeral init containers with their own registry image. They have no pull
// policy of their own, so we inherit the parent service's policy for skip
// decisions — through the same shouldPullImage decision as the service image.
// Unlike the service image, a hook image can't be built, so `build` falls
// back to pull-if-missing instead of exempting it from pulling.
func (p *imagePuller) pullHookImages(ctx context.Context) error {
	for name, service := range p.project.Services {
		hookPolicy := service.PullPolicy
		if hookPolicy == types.PullPolicyBuild {
			hookPolicy = types.PullPolicyMissing
		}
		for _, img := range api.GetDependentImages(service, p.project.Name) {
			pullRequired, skipReason, err := shouldPullImage(types.ServiceConfig{Name: name, ContainerSpec: types.ContainerSpec{Image: img, PullPolicy: hookPolicy}}, p.images)
			if err != nil {
				return err
			}
			if !pullRequired {
				if skipReason != "" {
					p.eventSkippedPull("Image "+img, skipReason)
				}
				continue
			}
			if _, ok := p.scheduled[img]; ok {
				continue
			}
			p.scheduled[img] = name
			hookService := types.ServiceConfig{Name: name, ContainerSpec: types.ContainerSpec{Image: img}}
			p.eg.Go(func() error {
				err := p.pullServiceImage(ctx, hookService, p.opts.Quiet, p.project.Environment["DOCKER_DEFAULT_PLATFORM"])
				if err != nil && !p.opts.IgnoreFailures {
					// fail fast: a hook image can't be built as a fallback
					return err
				}
				return nil
			})
		}
	}
	return nil
}

func (p *imagePuller) eventSkippedPull(id, details string) {
	p.events.On(api.Resource{
		ID:      id,
		Status:  api.Done,
		Text:    "Skipped",
		Details: details,
	})
}

// shouldPullImage decides whether `compose pull` refreshes a service's image,
// delegating to the exact pull_policy interpreter the up path uses (mustPull)
// so both commands honor never/build and skip-if-present identically. The
// command keeps deliberate differences reflecting that the user explicitly
// asked for a pull:
//   - a service without an explicit pull_policy is always refreshed — an
//     unset policy resolves to "missing" for `up`, but skipping it would turn
//     an explicit `compose pull` into a no-op once images exist;
//   - a daily/weekly/every_N refresh window is treated as due — an explicit
//     `compose pull` is the only way to force a refresh ahead of the window;
//   - a present `latest` tag is still refreshed under missing/if_not_present:
//     the tag is expected to move, and triggering the pull lets the daemon
//     negotiate with the registry — a manifest check, no download, when the
//     local image is already up to date;
//   - a service declaring both `provider:` and `image:` still gets its image
//     refreshed — mustPull leaves provider-managed services alone on the `up`
//     path, but an explicitly declared image remains pullable.
func shouldPullImage(service types.ServiceConfig, images map[string]api.ImageSummary) (bool, string, error) {
	if service.Provider != nil {
		if service.Image == "" {
			return false, "", nil
		}
		// neutralize the provider so mustPull doesn't short-circuit the
		// declared image
		service.Provider = nil
	}
	if service.PullPolicy == "" {
		return true, "", nil
	}
	pull, err := mustPull(service, images)
	if err != nil || pull {
		return pull, "", err
	}
	policy, _, _ := service.GetPullPolicy()
	switch policy {
	case types.PullPolicyRefresh:
		// the window would skip it on `up`, but an explicit pull is due now
		return true, "", nil
	case types.PullPolicyMissing, types.PullPolicyIfNotPresent:
		if isLatestTag(service.Image) {
			return true, "", nil
		}
		return false, "Image is already present locally", nil
	default: // never, build
		return false, "", nil
	}
}

// isLatestTag reports whether ref points at a `latest` tag, including bare
// references that normalize to it.
func isLatestTag(ref string) bool {
	named, err := reference.ParseDockerRef(ref)
	if err != nil {
		return false
	}
	tagged, ok := named.(reference.Tagged)
	return ok && tagged.Tag() == "latest"
}

func getUnwrappedErrorMessage(err error) string {
	derr := errors.Unwrap(err)
	if derr != nil {
		return getUnwrappedErrorMessage(derr)
	}
	return err.Error()
}

func (s *composeService) pullServiceImage(ctx context.Context, service types.ServiceConfig, quietPull bool, defaultPlatform string) error {
	resource := "Image " + service.Image
	s.events.On(newEvent(resource, api.Working, api.StatusPulling))
	ref, err := reference.ParseNormalizedNamed(service.Image)
	if err != nil {
		return err
	}

	encodedAuth, err := encodedAuth(ref, s.configFile())
	if err != nil {
		return err
	}

	platform := service.Platform
	if platform == "" {
		platform = defaultPlatform
	}

	var ociPlatforms []ocispec.Platform
	if platform != "" {
		p, err := platforms.Parse(platform)
		if err != nil {
			return err
		}
		ociPlatforms = append(ociPlatforms, p)
	}

	stream, err := s.apiClient().ImagePull(ctx, service.Image, client.ImagePullOptions{
		RegistryAuth: encodedAuth,
		Platforms:    ociPlatforms,
	})

	if ctx.Err() != nil {
		s.events.On(api.Resource{
			ID:     resource,
			Status: api.Warning,
			Text:   "Interrupted",
		})
		return nil
	}

	// check if it has an error and the service has a build section
	// then the status should be warning instead of error
	if err != nil && service.Build != nil {
		s.events.On(api.Resource{
			ID:     resource,
			Status: api.Warning,
			Text:   getUnwrappedErrorMessage(err),
		})
		return err
	}

	if err != nil {
		s.events.On(errorEvent(resource, getUnwrappedErrorMessage(err)))
		return err
	}

	dec := json.NewDecoder(stream)
	for {
		var jm jsonstream.Message
		if err := dec.Decode(&jm); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if jm.Error != nil {
			return errors.New(jm.Error.Message)
		}
		if !quietPull {
			toPullProgressEvent(resource, jm, s.events)
		}
	}
	s.events.On(newEvent(resource, api.Done, api.StatusPulled))

	return nil
}

// ImageDigestResolver creates a func able to resolve image digest from a
// docker ref, for pinning image references in a reproducible compose model
// (`compose publish` / `config --resolve-image-digests`).
//
// It deliberately returns the registry descriptor digest — the multi-platform
// index digest for multi-arch images — via DistributionInspect: a published
// compose file must stay deployable on any platform. This is NOT the same
// digest kind as localContentDigest, which selects the platform-specific
// runnable manifest to compare a running container with a fresh build/pull;
// never funnel this resolution through the local content-digest producer, and
// never pin a published reference with a per-platform digest.
func ImageDigestResolver(ctx context.Context, file *configfile.ConfigFile, apiClient client.APIClient) func(named reference.Named) (digest.Digest, error) {
	return func(named reference.Named) (digest.Digest, error) {
		auth, err := encodedAuth(named, file)
		if err != nil {
			return "", err
		}
		inspect, err := apiClient.DistributionInspect(ctx, named.String(), client.DistributionInspectOptions{
			EncodedRegistryAuth: auth,
		})
		if err != nil {
			return "",
				fmt.Errorf("failed to resolve digest for %s: %w", named.String(), err)
		}
		return inspect.Descriptor.Digest, nil
	}
}

type authProvider interface {
	GetAuthConfig(registryHostname string) (clitypes.AuthConfig, error)
}

func encodedAuth(ref reference.Named, configFile authProvider) (string, error) {
	authConfig, err := configFile.GetAuthConfig(registry.GetAuthConfigKey(reference.Domain(ref)))
	if err != nil {
		return "", err
	}

	buf, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}

func (s *composeService) pullRequiredImages(ctx context.Context, project *types.Project, images map[string]api.ImageSummary, quietPull bool) error {
	needPull := map[string]types.ServiceConfig{}
	// track image references already scheduled for pull so dependent images
	// (volume/hook images shared across services) aren't pulled more than once
	scheduled := map[string]bool{}
	for name, service := range project.Services {
		pull, err := mustPull(service, images)
		if err != nil {
			return err
		}
		if pull {
			needPull[name] = service
			scheduled[service.Image] = true
		}
		for i, vol := range service.Volumes {
			if vol.Type == types.VolumeTypeImage {
				if _, ok := images[vol.Source]; !ok {
					// Hack: create a fake ServiceConfig so we pull missing volume image
					n := fmt.Sprintf("%s:volume %d", name, i)
					needPull[n] = types.ServiceConfig{
						Name:          n,
						ContainerSpec: types.ContainerSpec{Image: vol.Source},
					}
					scheduled[vol.Source] = true
				}
			}
		}
	}

	if err := addPreStartHookPulls(project, images, needPull, scheduled); err != nil {
		return err
	}

	if len(needPull) == 0 {
		return nil
	}

	// the errgroup context is canceled as soon as Wait returns; the post-pull
	// resolution below needs the caller's context
	eg, pullCtx := errgroup.WithContext(ctx)
	eg.SetLimit(s.maxConcurrency)
	pulled := map[string]bool{}
	var mutex sync.Mutex
	for name, service := range needPull {
		eg.Go(func() error {
			err := s.pullServiceImage(pullCtx, service, quietPull, project.Environment["DOCKER_DEFAULT_PLATFORM"])
			mutex.Lock()
			defer mutex.Unlock()
			pulled[name] = err == nil
			if err != nil && isServiceImageToBuild(service, project.Services) {
				// image can be built, so we can ignore pull failure
				return nil
			}
			return err
		})
	}
	err := eg.Wait()
	return errors.Join(err, s.resolvePulledImages(ctx, needPull, pulled, images))
}

// resolvePulledImages resolves each distinct successfully pulled image once,
// the exact way getLocalImagesDigests does for already-local images — both
// values feed the com.docker.compose.image label used to detect stale
// containers, so they must be computed identically, for the host default
// platform. Resolving from each pull's goroutine (or for the pulled platform)
// let the recorded digest depend on pull completion order when several
// services pull the same tag for different platforms. Platform-pinned
// services get the digest of their own platform's manifest from
// serviceImageDigest when the label is written.
func (s *composeService) resolvePulledImages(ctx context.Context, needPull map[string]types.ServiceConfig, pulled map[string]bool, images map[string]api.ImageSummary) error {
	var errs error
	resolved := map[string]bool{}
	for name, service := range needPull {
		if !pulled[name] || resolved[service.Image] {
			continue
		}
		resolved[service.Image] = true
		id := "dryRunId"
		if !s.dryRun {
			// in dry-run the image was never actually pulled, so there is
			// nothing to inspect locally
			var ierr error
			id, _, ierr = s.inspectLocalContent(ctx, service.Image, "")
			if ierr != nil {
				errs = errors.Join(errs, ierr)
				continue
			}
		}
		images[service.Image] = api.ImageSummary{
			ID:          id,
			Repository:  service.Image,
			LastTagTime: time.Now(),
		}
	}
	return errs
}

// addPreStartHookPulls schedules pulls for pre_start hook images.
// pre_start hooks run as ephemeral init containers with their own registry
// image; they have no pull policy of their own, so they inherit the parent
// service's — interpreted by the same mustPull the service image goes
// through, so `up` and `compose pull` agree on hook images too. A hook image
// can't be built, so `build` falls back to pull-if-missing instead of
// exempting it from pulling; only `never` skips entirely. Running as a second
// pass over the services keeps the dedup against service/volume images (via
// scheduled) independent of service iteration order.
func addPreStartHookPulls(project *types.Project, images map[string]api.ImageSummary, needPull map[string]types.ServiceConfig, scheduled map[string]bool) error {
	for name, service := range project.Services {
		if service.PullPolicy == types.PullPolicyNever {
			continue
		}
		hookPolicy := service.PullPolicy
		if hookPolicy == types.PullPolicyBuild {
			hookPolicy = types.PullPolicyMissing
		}
		for i, img := range api.GetDependentImages(service, project.Name) {
			pull, err := mustPull(types.ServiceConfig{Name: name, ContainerSpec: types.ContainerSpec{Image: img, PullPolicy: hookPolicy}}, images)
			if err != nil {
				return err
			}
			if !pull || scheduled[img] {
				continue
			}
			scheduled[img] = true
			// Hack: create a fake ServiceConfig so we pull missing pre_start hook image
			n := fmt.Sprintf("%s:pre_start %d", name, i)
			needPull[n] = types.ServiceConfig{
				Name:          n,
				ContainerSpec: types.ContainerSpec{Image: img},
			}
		}
	}
	return nil
}

func mustPull(service types.ServiceConfig, images map[string]api.ImageSummary) (bool, error) {
	if service.Provider != nil {
		return false, nil
	}
	if service.Image == "" {
		return false, nil
	}
	policy, duration, err := service.GetPullPolicy()
	if err != nil {
		return false, err
	}
	switch policy {
	case types.PullPolicyAlways:
		// force pull
		return true, nil
	case types.PullPolicyNever, types.PullPolicyBuild:
		return false, nil
	case types.PullPolicyRefresh:
		img, ok := images[service.Image]
		if !ok {
			return true, nil
		}
		return time.Now().After(img.LastTagTime.Add(duration)), nil
	default: // Pull if missing
		_, ok := images[service.Image]
		return !ok, nil
	}
}

func isServiceImageToBuild(service types.ServiceConfig, services types.Services) bool {
	if service.Build != nil {
		return true
	}

	if service.Image == "" {
		// N.B. this should be impossible as service must have either `build` or `image` (or both)
		return false
	}

	// look through the other services to see if another has a build definition for the same
	// image name
	for _, svc := range services {
		if svc.Image == service.Image && svc.Build != nil {
			return true
		}
	}
	return false
}

const (
	PreparingPhase         = "Preparing"
	WaitingPhase           = "waiting"
	PullingFsPhase         = "Pulling fs layer"
	DownloadingPhase       = "Downloading"
	DownloadCompletePhase  = "Download complete"
	ExtractingPhase        = "Extracting"
	VerifyingChecksumPhase = "Verifying Checksum"
	AlreadyExistsPhase     = "Already exists"
	PullCompletePhase      = "Pull complete"
)

func toPullProgressEvent(parent string, jm jsonstream.Message, events api.EventProcessor) {
	if jm.ID == "" || jm.Progress == nil {
		return
	}

	var (
		details string
		total   int64
		percent int
		current int64
		status  = api.Working
	)

	switch jm.Status {
	case PreparingPhase, WaitingPhase, PullingFsPhase:
		percent = 0
	case DownloadingPhase, ExtractingPhase, VerifyingChecksumPhase:
		if jm.Progress != nil {
			current = jm.Progress.Current
			total = jm.Progress.Total
			if jm.Progress.Total > 0 {
				percent = min(int(jm.Progress.Current*100/jm.Progress.Total), 100)
			}
		}
	case DownloadCompletePhase, AlreadyExistsPhase, PullCompletePhase:
		status = api.Done
		percent = 100
	}

	if strings.Contains(jm.Status, "Image is up to date") ||
		strings.Contains(jm.Status, "Downloaded newer image") {
		status = api.Done
		percent = 100
	}

	if jm.Error != nil {
		status = api.Error
		details = jm.Error.Message
	} else {
		details = units.HumanSize(float64(jm.Progress.Current))
	}

	events.On(api.Resource{
		ID:       jm.ID,
		ParentID: parent,
		Current:  current,
		Total:    total,
		Percent:  percent,
		Status:   status,
		Text:     jm.Status,
		Details:  details,
	})
}
