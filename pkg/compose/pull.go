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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/distribution/reference"
	"github.com/docker/buildx/driver"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/registry"
	"github.com/hashicorp/go-multierror"
	"github.com/opencontainers/go-digest"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
)

func (s *composeService) Pull(ctx context.Context, project *types.Project, options api.PullOptions) error {
	if options.Quiet {
		return s.pull(ctx, project, options)
	}
	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		return s.pull(ctx, project, options)
	}, s.stdinfo(), "Pulling")
}

func (s *composeService) pull(ctx context.Context, project *types.Project, opts api.PullOptions) error { //nolint:gocyclo
	images, err := s.getLocalImagesDigests(ctx, project)
	if err != nil {
		return err
	}

	w := progress.ContextWriter(ctx)
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
			w.Event(progress.Event{
				ID:     name,
				Status: progress.Done,
				Text:   "Skipped - No image to be pulled",
			})
			continue
		}

		switch service.PullPolicy {
		case types.PullPolicyNever, types.PullPolicyBuild:
			w.Event(progress.Event{
				ID:     name,
				Status: progress.Done,
				Text:   "Skipped",
			})
			continue
		case types.PullPolicyMissing, types.PullPolicyIfNotPresent:
			if imageAlreadyPresent(service.Image, images) {
				w.Event(progress.Event{
					ID:     name,
					Status: progress.Done,
					Text:   "Skipped - Image is already present locally",
				})
				continue
			}
		}

		if service.Build != nil && opts.IgnoreBuildable {
			w.Event(progress.Event{
				ID:     name,
				Status: progress.Done,
				Text:   "Skipped - Image can be built",
			})
			continue
		}

		if s, ok := imagesBeingPulled[service.Image]; ok {
			w.Event(progress.Event{
				ID:     name,
				Status: progress.Done,
				Text:   fmt.Sprintf("Skipped - Image is already being pulled by %v", s),
			})
			continue
		}

		imagesBeingPulled[service.Image] = service.Name

		idx, name, service := i, name, service
		eg.Go(func() error {
			_, err := s.pullServiceImage(ctx, service, s.configFile(), w, false, project.Environment["DOCKER_DEFAULT_PLATFORM"])
			if err != nil {
				pullErrors[idx] = err
				if service.Build != nil {
					mustBuild = append(mustBuild, service.Name)
				}
				if !opts.IgnoreFailures && service.Build == nil {
					if s.dryRun {
						w.Event(progress.Event{
							ID:     name,
							Status: progress.Error,
							Text:   fmt.Sprintf(" - Pull error for image: %s", service.Image),
						})
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
		w.TailMsgf("WARNING: Some service image(s) must be built from source by running:\n    docker compose build %s", strings.Join(mustBuild, " "))
	}

	if err != nil {
		return err
	}
	if opts.IgnoreFailures {
		return nil
	}
	return multierror.Append(nil, pullErrors...).ErrorOrNil()
}

func imageAlreadyPresent(serviceImage string, localImages map[string]string) bool {
	normalizedImage, err := reference.ParseDockerRef(serviceImage)
	if err != nil {
		return false
	}
	tagged, ok := normalizedImage.(reference.NamedTagged)
	if !ok {
		return false
	}
	_, ok = localImages[serviceImage]
	return ok && tagged.Tag() != "latest"
}

func getUnwrappedErrorMessage(err error) string {
	derr := errors.Unwrap(err)
	if derr != nil {
		return getUnwrappedErrorMessage(derr)
	}
	return err.Error()
}

func (s *composeService) pullServiceImage(ctx context.Context, service types.ServiceConfig,
	configFile driver.Auth, w progress.Writer, quietPull bool, defaultPlatform string) (string, error) {
	w.Event(progress.Event{
		ID:     service.Name,
		Status: progress.Working,
		Text:   "Pulling",
	})
	ref, err := reference.ParseNormalizedNamed(service.Image)
	if err != nil {
		return "", err
	}

	encodedAuth, err := encodedAuth(ref, configFile)
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

	// check if has error and the service has a build section
	// then the status should be warning instead of error
	if err != nil && service.Build != nil {
		w.Event(progress.Event{
			ID:         service.Name,
			Status:     progress.Warning,
			Text:       "Warning",
			StatusText: getUnwrappedErrorMessage(err),
		})
		return "", WrapCategorisedComposeError(err, PullFailure)
	}

	if err != nil {
		w.Event(progress.Event{
			ID:         service.Name,
			Status:     progress.Error,
			Text:       "Error",
			StatusText: getUnwrappedErrorMessage(err),
		})
		return "", WrapCategorisedComposeError(err, PullFailure)
	}

	dec := json.NewDecoder(stream)
	for {
		var jm jsonmessage.JSONMessage
		if err := dec.Decode(&jm); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", WrapCategorisedComposeError(err, PullFailure)
		}
		if jm.Error != nil {
			return "", WrapCategorisedComposeError(errors.New(jm.Error.Message), PullFailure)
		}
		if !quietPull {
			toPullProgressEvent(service.Name, jm, w)
		}
	}
	w.Event(progress.Event{
		ID:     service.Name,
		Status: progress.Done,
		Text:   "Pulled",
	})

	inspected, _, err := s.apiClient().ImageInspectWithRaw(ctx, service.Image)
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
				fmt.Errorf("failed ot resolve digest for %s: %w", named.String(), err)
		}
		return inspect.Descriptor.Digest, nil
	}
}

func encodedAuth(ref reference.Named, configFile driver.Auth) (string, error) {
	repoInfo, err := registry.ParseRepositoryInfo(ref)
	if err != nil {
		return "", err
	}

	key := registry.GetAuthConfigKey(repoInfo.Index)
	authConfig, err := configFile.GetAuthConfig(key)
	if err != nil {
		return "", err
	}

	buf, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}

func (s *composeService) pullRequiredImages(ctx context.Context, project *types.Project, images map[string]string, quietPull bool) error {
	var needPull []types.ServiceConfig
	for _, service := range project.Services {
		if service.Image == "" {
			continue
		}
		switch service.PullPolicy {
		case "", types.PullPolicyMissing, types.PullPolicyIfNotPresent:
			if _, ok := images[service.Image]; ok {
				continue
			}
		case types.PullPolicyNever, types.PullPolicyBuild:
			continue
		case types.PullPolicyAlways:
			// force pull
		}
		needPull = append(needPull, service)
	}
	if len(needPull) == 0 {
		return nil
	}

	return progress.Run(ctx, func(ctx context.Context) error {
		w := progress.ContextWriter(ctx)
		eg, ctx := errgroup.WithContext(ctx)
		eg.SetLimit(s.maxConcurrency)
		pulledImages := make([]string, len(needPull))
		for i, service := range needPull {
			i, service := i, service
			eg.Go(func() error {
				id, err := s.pullServiceImage(ctx, service, s.configFile(), w, quietPull, project.Environment["DOCKER_DEFAULT_PLATFORM"])
				pulledImages[i] = id
				if err != nil && isServiceImageToBuild(service, project.Services) {
					// image can be built, so we can ignore pull failure
					return nil
				}
				return err
			})
		}
		err := eg.Wait()
		for i, service := range needPull {
			if pulledImages[i] != "" {
				images[service.Image] = pulledImages[i]
			}
		}
		return err
	}, s.stdinfo())
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
	WaitingPhase           = "Waiting"
	PullingFsPhase         = "Pulling fs layer"
	DownloadingPhase       = "Downloading"
	DownloadCompletePhase  = "Download complete"
	ExtractingPhase        = "Extracting"
	VerifyingChecksumPhase = "Verifying Checksum"
	AlreadyExistsPhase     = "Already exists"
	PullCompletePhase      = "Pull complete"
)

func toPullProgressEvent(parent string, jm jsonmessage.JSONMessage, w progress.Writer) {
	if jm.ID == "" || jm.Progress == nil {
		return
	}

	var (
		text    string
		total   int64
		percent int
		current int64
		status  = progress.Working
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
		status = progress.Done
		percent = 100
	}

	if strings.Contains(jm.Status, "Image is up to date") ||
		strings.Contains(jm.Status, "Downloaded newer image") {
		status = progress.Done
		percent = 100
	}

	if jm.Error != nil {
		status = progress.Error
		text = jm.Error.Message
	}

	w.Event(progress.Event{
		ID:         jm.ID,
		ParentID:   parent,
		Current:    current,
		Total:      total,
		Percent:    percent,
		Text:       jm.Status,
		Status:     status,
		StatusText: text,
	})
}
