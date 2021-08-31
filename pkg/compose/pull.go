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
	"io"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/distribution/distribution/v3/reference"
	"github.com/docker/buildx/driver"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/registry"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
)

func (s *composeService) Pull(ctx context.Context, project *types.Project, opts api.PullOptions) error {
	if opts.Quiet {
		return s.pull(ctx, project, opts)
	}
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.pull(ctx, project, opts)
	})
}

func (s *composeService) pull(ctx context.Context, project *types.Project, opts api.PullOptions) error {
	info, err := s.apiClient.Info(ctx)
	if err != nil {
		return err
	}

	if info.IndexServerAddress == "" {
		info.IndexServerAddress = registry.IndexServer
	}

	w := progress.ContextWriter(ctx)
	eg, ctx := errgroup.WithContext(ctx)

	var mustBuild []string
	for _, service := range project.Services {
		service := service
		if service.Image == "" {
			w.Event(progress.Event{
				ID:     service.Name,
				Status: progress.Done,
				Text:   "Skipped",
			})
			continue
		}
		eg.Go(func() error {
			err := s.pullServiceImage(ctx, service, info, s.configFile, w, false)
			if err != nil {
				if !opts.IgnoreFailures {
					if service.Build != nil {
						mustBuild = append(mustBuild, service.Name)
					}
					return err
				}
				w.TailMsgf("Pulling %s: %s", service.Name, err.Error())
			}
			return nil
		})
	}

	err = eg.Wait()

	if !opts.IgnoreFailures && len(mustBuild) > 0 {
		w.TailMsgf("WARNING: Some service image(s) must be built from source by running:\n    docker compose build %s", strings.Join(mustBuild, " "))
	}

	return err
}

func (s *composeService) pullServiceImage(ctx context.Context, service types.ServiceConfig, info moby.Info, configFile driver.Auth, w progress.Writer, quietPull bool) error {
	w.Event(progress.Event{
		ID:     service.Name,
		Status: progress.Working,
		Text:   "Pulling",
	})
	ref, err := reference.ParseNormalizedNamed(service.Image)
	if err != nil {
		return err
	}

	repoInfo, err := registry.ParseRepositoryInfo(ref)
	if err != nil {
		return err
	}

	key := repoInfo.Index.Name
	if repoInfo.Index.Official {
		key = info.IndexServerAddress
	}

	authConfig, err := configFile.GetAuthConfig(key)
	if err != nil {
		return err
	}

	buf, err := json.Marshal(authConfig)
	if err != nil {
		return err
	}

	stream, err := s.apiClient.ImagePull(ctx, service.Image, moby.ImagePullOptions{
		RegistryAuth: base64.URLEncoding.EncodeToString(buf),
		Platform:     service.Platform,
	})
	if err != nil {
		w.Event(progress.Event{
			ID:     service.Name,
			Status: progress.Error,
			Text:   "Error",
		})
		return WrapCategorisedComposeError(err, PullFailure)
	}

	dec := json.NewDecoder(stream)
	for {
		var jm jsonmessage.JSONMessage
		if err := dec.Decode(&jm); err != nil {
			if err == io.EOF {
				break
			}
			return WrapCategorisedComposeError(err, PullFailure)
		}
		if jm.Error != nil {
			return WrapCategorisedComposeError(errors.New(jm.Error.Message), PullFailure)
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
	return nil
}

func (s *composeService) pullRequiredImages(ctx context.Context, project *types.Project, images map[string]string, quietPull bool) error {
	info, err := s.apiClient.Info(ctx)
	if err != nil {
		return err
	}

	if info.IndexServerAddress == "" {
		info.IndexServerAddress = registry.IndexServer
	}

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
		for _, service := range needPull {
			service := service
			eg.Go(func() error {
				err := s.pullServiceImage(ctx, service, info, s.configFile, w, quietPull)
				if err != nil && service.Build != nil {
					// image can be built, so we can ignore pull failure
					return nil
				}
				return err
			})
		}
		return eg.Wait()
	})
}

func toPullProgressEvent(parent string, jm jsonmessage.JSONMessage, w progress.Writer) {
	if jm.ID == "" || jm.Progress == nil {
		return
	}

	var (
		text   string
		status = progress.Working
	)

	text = jm.Progress.String()

	if jm.Status == "Pull complete" ||
		jm.Status == "Already exists" ||
		strings.Contains(jm.Status, "Image is up to date") ||
		strings.Contains(jm.Status, "Downloaded newer image") {
		status = progress.Done
	}

	if jm.Error != nil {
		status = progress.Error
		text = jm.Error.Message
	}

	w.Event(progress.Event{
		ID:         jm.ID,
		ParentID:   parent,
		Text:       jm.Status,
		Status:     status,
		StatusText: text,
	})
}
