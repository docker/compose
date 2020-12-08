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
	cliconfig "github.com/docker/cli/cli/config"
	"github.com/docker/distribution/reference"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/registry"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose-cli/config"
	"github.com/docker/compose-cli/progress"
)

func (s *composeService) Pull(ctx context.Context, project *types.Project) error {
	configFile, err := cliconfig.Load(config.Dir(ctx))
	if err != nil {
		return err
	}
	info, err := s.apiClient.Info(ctx)
	if err != nil {
		return err
	}

	if info.IndexServerAddress == "" {
		info.IndexServerAddress = registry.IndexServer
	}

	w := progress.ContextWriter(ctx)
	eg, ctx := errgroup.WithContext(ctx)

	for _, srv := range project.Services {
		service := srv
		eg.Go(func() error {
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
			})
			if err != nil {
				w.Event(progress.Event{
					ID:     service.Name,
					Status: progress.Error,
					Text:   "Error",
				})
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
				toPullProgressEvent(service.Name, jm, w)
			}
			w.Event(progress.Event{
				ID:     service.Name,
				Status: progress.Done,
				Text:   "Pulled",
			})
			return nil
		})
	}

	return eg.Wait()
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
