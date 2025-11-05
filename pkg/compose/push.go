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
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/pkg/jsonmessage"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/internal/registry"
	"github.com/docker/compose/v2/pkg/api"
)

func (s *composeService) Push(ctx context.Context, project *types.Project, options api.PushOptions) error {
	if options.Quiet {
		return s.push(ctx, project, options)
	}
	return Run(ctx, func(ctx context.Context) error {
		return s.push(ctx, project, options)
	}, "push", s.events)
}

func (s *composeService) push(ctx context.Context, project *types.Project, options api.PushOptions) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(s.maxConcurrency)

	for _, service := range project.Services {
		if service.Build == nil || service.Image == "" {
			if options.ImageMandatory && service.Image == "" && service.Provider == nil {
				return fmt.Errorf("%q attribute is mandatory to push an image for service %q", "service.image", service.Name)
			}
			s.events.On(api.Resource{
				ID:     service.Name,
				Status: api.Done,
				Text:   "Skipped",
			})
			continue
		}
		tags := []string{service.Image}
		if service.Build != nil {
			tags = append(tags, service.Build.Tags...)
		}

		for _, tag := range tags {
			eg.Go(func() error {
				s.events.On(newEvent(tag, api.Working, "Pushing"))
				err := s.pushServiceImage(ctx, tag, options.Quiet)
				if err != nil {
					if !options.IgnoreFailures {
						s.events.On(newEvent(tag, api.Error, err.Error()))
						return err
					}
					s.events.On(newEvent(tag, api.Warning, err.Error()))
				} else {
					s.events.On(newEvent(tag, api.Done, "Pushed"))
				}
				return nil
			})
		}
	}
	return eg.Wait()
}

func (s *composeService) pushServiceImage(ctx context.Context, tag string, quietPush bool) error {
	ref, err := reference.ParseNormalizedNamed(tag)
	if err != nil {
		return err
	}

	authConfig, err := s.configFile().GetAuthConfig(registry.GetAuthConfigKey(reference.Domain(ref)))
	if err != nil {
		return err
	}

	buf, err := json.Marshal(authConfig)
	if err != nil {
		return err
	}

	stream, err := s.apiClient().ImagePush(ctx, tag, image.PushOptions{
		RegistryAuth: base64.URLEncoding.EncodeToString(buf),
	})
	if err != nil {
		return err
	}
	dec := json.NewDecoder(stream)
	for {
		var jm jsonmessage.JSONMessage
		if err := dec.Decode(&jm); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if jm.Error != nil {
			return errors.New(jm.Error.Message)
		}

		if !quietPush {
			toPushProgressEvent(tag, jm, s.events)
		}
	}

	return nil
}

func toPushProgressEvent(prefix string, jm jsonmessage.JSONMessage, events api.EventProcessor) {
	if jm.ID == "" {
		// skipped
		return
	}
	var (
		text    string
		status  = api.Working
		total   int64
		current int64
		percent int
	)
	if isDone(jm) {
		status = api.Done
		percent = 100
	}
	if jm.Error != nil {
		status = api.Error
		text = jm.Error.Message
	}
	if jm.Progress != nil {
		text = jm.Progress.String()
		if jm.Progress.Total != 0 {
			current = jm.Progress.Current
			total = jm.Progress.Total
			if jm.Progress.Total > 0 {
				percent = int(jm.Progress.Current * 100 / jm.Progress.Total)
			}
		}
	}

	events.On(api.Resource{
		ParentID: prefix,
		ID:       jm.ID,
		Text:     text,
		Status:   status,
		Current:  current,
		Total:    total,
		Percent:  percent,
	})
}

func isDone(msg jsonmessage.JSONMessage) bool {
	// TODO there should be a better way to detect push is done than such a status message check
	switch strings.ToLower(msg.Status) {
	case "pushed", "layer already exists":
		return true
	default:
		return false
	}
}
