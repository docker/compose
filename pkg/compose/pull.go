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
	"github.com/distribution/reference"
	"github.com/docker/buildx/driver"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/internal/registry"
	"github.com/docker/compose/v2/pkg/api"
)

func (s *composeService) Pull(ctx context.Context, project *types.Project, options api.PullOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.pull(ctx, project, options)
	}, "pull", s.events)
}

func (s *composeService) pull(ctx context.Context, project *types.Project, opts api.PullOptions) error { //nolint:gocyclo
	images, err := s.getLocalImagesDigests(ctx, project)
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(s.maxConcurrency)

	var (
		mustBuild         []string
		pullErrors        = make([]error, len(project.Services))
		imagesBeingPulled = map[string]string{}
	)

	i := 0
	for name, service := range project.Services {
		if service.Image == "" {
			s.events.On(api.Resource{
				ID:      name,
				Status:  api.Done,
				Text:    "Skipped",
				Details: "No image to be pulled",
			})
			continue
		}

		switch service.PullPolicy {
		case types.PullPolicyNever, types.PullPolicyBuild:
			s.events.On(api.Resource{
				ID:     "Image " + service.Image,
				Status: api.Done,
				Text:   "Skipped",
			})
			continue
		case types.PullPolicyMissing, types.PullPolicyIfNotPresent:
			if imageAlreadyPresent(service.Image, images) {
				s.events.On(api.Resource{
					ID:      "Image " + service.Image,
					Status:  api.Done,
					Text:    "Skipped",
					Details: "Image is already present locally",
				})
				continue
			}
		}

		if service.Build != nil && opts.IgnoreBuildable {
			s.events.On(api.Resource{
				ID:      "Image " + service.Image,
				Status:  api.Done,
				Text:    "Skipped",
				Details: "Image can be built",
			})
			continue
		}

		if _, ok := imagesBeingPulled[service.Image]; ok {
			continue
		}

		imagesBeingPulled[service.Image] = service.Name

		idx := i
		eg.Go(func() error {
			_, err := s.pullServiceImage(ctx, service, opts.Quiet, project.Environment["DOCKER_DEFAULT_PLATFORM"])
			if err != nil {
				pullErrors[idx] = err
				if service.Build != nil {
					mustBuild = append(mustBuild, service.Name)
				}
				if !opts.IgnoreFailures && service.Build == nil {
					if s.dryRun {
						s.events.On(errorEventf("Image "+service.Image,
							"error pulling image: %s", service.Image))
					}
					// fail fast if image can't be pulled nor built
					return err
				}
			}
			return nil
		})
		i++
	}

	err = eg.Wait()

	if len(mustBuild) > 0 {
		logrus.Warnf("WARNING: Some service image(s) must be built from source by running:\n    docker compose build %s", strings.Join(mustBuild, " "))
	}

	if err != nil {
		return err
	}
	if opts.IgnoreFailures {
		return nil
	}
	return errors.Join(pullErrors...)
}

func imageAlreadyPresent(serviceImage string, localImages map[string]api.ImageSummary) bool {
	normalizedImage, err := reference.ParseDockerRef(serviceImage)
	if err != nil {
		return false
	}
	switch refType := normalizedImage.(type) {
	case reference.NamedTagged:
		_, ok := localImages[serviceImage]
		return ok && refType.Tag() != "latest"
	default:
		_, ok := localImages[serviceImage]
		return ok
	}
}

func getUnwrappedErrorMessage(err error) string {
	derr := errors.Unwrap(err)
	if derr != nil {
		return getUnwrappedErrorMessage(derr)
	}
	return err.Error()
}

func (s *composeService) pullServiceImage(ctx context.Context, service types.ServiceConfig, quietPull bool, defaultPlatform string) (string, error) {
	resource := "Image " + service.Image
	s.events.On(pullingEvent(service.Image))
	ref, err := reference.ParseNormalizedNamed(service.Image)
	if err != nil {
		return "", err
	}

	encodedAuth, err := encodedAuth(ref, s.configFile())
	if err != nil {
		return "", err
	}

	platform := service.Platform
	if platform == "" {
		platform = defaultPlatform
	}

	stream, err := s.apiClient().ImagePull(ctx, service.Image, image.PullOptions{
		RegistryAuth: encodedAuth,
		Platform:     platform,
	})

	if ctx.Err() != nil {
		s.events.On(api.Resource{
			ID:     resource,
			Status: api.Warning,
			Text:   "Interrupted",
		})
		return "", nil
	}

	// check if has error and the service has a build section
	// then the status should be warning instead of error
	if err != nil && service.Build != nil {
		s.events.On(api.Resource{
			ID:     resource,
			Status: api.Warning,
			Text:   getUnwrappedErrorMessage(err),
		})
		return "", err
	}

	if err != nil {
		s.events.On(errorEvent(resource, getUnwrappedErrorMessage(err)))
		return "", err
	}

	dec := json.NewDecoder(stream)
	for {
		var jm jsonmessage.JSONMessage
		if err := dec.Decode(&jm); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
		if jm.Error != nil {
			return "", errors.New(jm.Error.Message)
		}
		if !quietPull {
			toPullProgressEvent(resource, jm, s.events)
		}
	}
	s.events.On(pulledEvent(service.Image))

	inspected, err := s.apiClient().ImageInspect(ctx, service.Image)
	if err != nil {
		return "", err
	}
	return inspected.ID, nil
}

// ImageDigestResolver creates a func able to resolve image digest from a docker ref,
func ImageDigestResolver(ctx context.Context, file *configfile.ConfigFile, apiClient client.APIClient) func(named reference.Named) (digest.Digest, error) {
	return func(named reference.Named) (digest.Digest, error) {
		auth, err := encodedAuth(named, file)
		if err != nil {
			return "", err
		}
		inspect, err := apiClient.DistributionInspect(ctx, named.String(), auth)
		if err != nil {
			return "",
				fmt.Errorf("failed to resolve digest for %s: %w", named.String(), err)
		}
		return inspect.Descriptor.Digest, nil
	}
}

func encodedAuth(ref reference.Named, configFile driver.Auth) (string, error) {
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
	for name, service := range project.Services {
		pull, err := mustPull(service, images)
		if err != nil {
			return err
		}
		if pull {
			needPull[name] = service
		}
		for i, vol := range service.Volumes {
			if vol.Type == types.VolumeTypeImage {
				if _, ok := images[vol.Source]; !ok {
					// Hack: create a fake ServiceConfig so we pull missing volume image
					n := fmt.Sprintf("%s:volume %d", name, i)
					needPull[n] = types.ServiceConfig{
						Name:  n,
						Image: vol.Source,
					}
				}
			}
		}

	}
	if len(needPull) == 0 {
		return nil
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(s.maxConcurrency)
	pulledImages := map[string]api.ImageSummary{}
	var mutex sync.Mutex
	for name, service := range needPull {
		eg.Go(func() error {
			id, err := s.pullServiceImage(ctx, service, quietPull, project.Environment["DOCKER_DEFAULT_PLATFORM"])
			mutex.Lock()
			defer mutex.Unlock()
			pulledImages[name] = api.ImageSummary{
				ID:          id,
				Repository:  service.Image,
				LastTagTime: time.Now(),
			}
			if err != nil && isServiceImageToBuild(service, project.Services) {
				// image can be built, so we can ignore pull failure
				return nil
			}
			return err
		})
	}
	err := eg.Wait()
	for i, service := range needPull {
		if pulledImages[i].ID != "" {
			images[service.Image] = pulledImages[i]
		}
	}
	return err
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

func toPullProgressEvent(parent string, jm jsonmessage.JSONMessage, events api.EventProcessor) {
	if jm.ID == "" || jm.Progress == nil {
		return
	}

	var (
		text    string
		total   int64
		percent int
		current int64
		status  = api.Working
	)

	text = jm.Progress.String()

	switch jm.Status {
	case PreparingPhase, WaitingPhase, PullingFsPhase:
		percent = 0
	case DownloadingPhase, ExtractingPhase, VerifyingChecksumPhase:
		if jm.Progress != nil {
			current = jm.Progress.Current
			total = jm.Progress.Total
			if jm.Progress.Total > 0 {
				percent = int(jm.Progress.Current * 100 / jm.Progress.Total)
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
		text = jm.Error.Message
	}

	events.On(api.Resource{
		ID:       jm.ID,
		ParentID: parent,
		Current:  current,
		Total:    total,
		Percent:  percent,
		Status:   status,
		Text:     text,
	})
}
