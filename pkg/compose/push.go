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
	"fmt"
	"io"

	"github.com/compose-spec/compose-go/types"
	"github.com/distribution/distribution/v3/reference"
	"github.com/docker/buildx/driver"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/registry"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
)

func (s *composeService) Push(ctx context.Context, project *types.Project, options api.PushOptions) error {
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.push(ctx, project, options)
	})
}

func (s *composeService) push(ctx context.Context, project *types.Project, options api.PushOptions) error {
	eg, ctx := errgroup.WithContext(ctx)

	info, err := s.apiClient.Info(ctx)
	if err != nil {
		return err
	}
	if info.IndexServerAddress == "" {
		info.IndexServerAddress = registry.IndexServer
	}

	w := progress.ContextWriter(ctx)
	for _, service := range project.Services {
		if service.Build == nil || service.Image == "" {
			w.Event(progress.Event{
				ID:     service.Name,
				Status: progress.Done,
				Text:   "Skipped",
			})
			continue
		}
		service := service
		eg.Go(func() error {
			err := s.pushServiceImage(ctx, service, info, s.configFile, w)
			if err != nil {
				if !options.IgnoreFailures {
					return err
				}
				w.TailMsgf("Pushing %s: %s", service.Name, err.Error())
			}
			return nil
		})
	}
	return eg.Wait()
}

func (s *composeService) pushServiceImage(ctx context.Context, service types.ServiceConfig, info moby.Info, configFile driver.Auth, w progress.Writer) error {
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

	stream, err := s.apiClient.ImagePush(ctx, service.Image, moby.ImagePushOptions{
		RegistryAuth: base64.URLEncoding.EncodeToString(buf),
	})
	if err != nil {
		return err
	}
	dec := json.NewDecoder(stream)
	for {
		var jm jsonmessage.JSONMessage
		if err := dec.Decode(&jm); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if jm.Error != nil {
			return errors.New(jm.Error.Message)
		}
		toPushProgressEvent(service.Name, jm, w)
	}
	return nil
}

func toPushProgressEvent(prefix string, jm jsonmessage.JSONMessage, w progress.Writer) {
	if jm.ID == "" {
		// skipped
		return
	}
	var (
		text   string
		status = progress.Working
	)
	if jm.Status == "Pull complete" || jm.Status == "Already exists" {
		status = progress.Done
	}
	if jm.Error != nil {
		status = progress.Error
		text = jm.Error.Message
	}
	if jm.Progress != nil {
		text = jm.Progress.String()
	}
	w.Event(progress.Event{
		ID:         fmt.Sprintf("Pushing %s: %s", prefix, jm.ID),
		Text:       jm.Status,
		Status:     status,
		StatusText: text,
	})
}
