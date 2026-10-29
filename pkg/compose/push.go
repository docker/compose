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
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/docker/go-units"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/internal/concurrency"
	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/internal/registry"
	"github.com/docker/compose/v5/pkg/api"
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
	eg, ctx := concurrency.NewErrgroup(ctx, s.maxConcurrency)

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
				if options.IgnoreMissing && s.isMissingLocally(ctx, tag) {
					s.events.On(progress.New(tag, api.Done, "Skipped", "No local image"))
					return nil
				}
				s.events.On(progress.New(tag, api.Working, "Pushing"))
				err := s.pushServiceImage(ctx, tag, options.Quiet)
				if err != nil {
					if !options.IgnoreFailures {
						s.events.On(progress.New(tag, api.Error, err.Error()))
						return err
					}
					s.events.On(progress.New(tag, api.Warning, err.Error()))
				} else {
					s.events.On(progress.New(tag, api.Done, "Pushed"))
				}
				return nil
			})
		}
	}
	return eg.Wait()
}

// isMissingLocally tells whether the image is known to be absent from the local image store; any other failure
// to look it up is left to the push to report.
func (s *composeService) isMissingLocally(ctx context.Context, tag string) bool {
	_, err := s.apiClient().ImageInspect(ctx, tag)
	return errdefs.IsNotFound(err)
}

func (s *composeService) pushServiceImage(ctx context.Context, tag string, quietPush bool) error {
	ref, err := reference.ParseNormalizedNamed(tag)
	if err != nil {
		return err
	}

	encodedAuth, err := registry.EncodedAuth(ref, s.authProvider(ctx))
	if err != nil {
		return err
	}

	stream, err := s.apiClient().ImagePush(ctx, tag, client.ImagePushOptions{
		RegistryAuth: encodedAuth,
	})
	if err != nil {
		return err
	}
	return decodeProgressStream(stream, func(jm jsonstream.Message) {
		if !quietPush {
			toPushProgressEvent(tag, jm, s.events)
		}
	})
}

func toPushProgressEvent(prefix string, jm jsonstream.Message, events api.EventProcessor) {
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
		text = progressText(jm.Progress)
		if jm.Progress.Total != 0 {
			current = jm.Progress.Current
			total = jm.Progress.Total
			if jm.Progress.Total > 0 {
				percent = progressPercent(jm.Progress.Current, jm.Progress.Total)
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

func isDone(msg jsonstream.Message) bool {
	// TODO there should be a better way to detect push is done than such a status message check
	switch strings.ToLower(msg.Status) {
	case "pushed", "layer already exists":
		return true
	default:
		return false
	}
}

// progressText is a minimal variant of [jsonmessage.JSONProgress.String()]
//
// [jsonmessage.JSONProgress.String()]: https://github.com/moby/moby/blob/v28.5.2/pkg/jsonmessage/jsonmessage.go#L54-L117
func progressText(p *jsonstream.Progress) string {
	switch {
	case p.Current <= 0 && p.Total <= 0:
		return ""
	case p.Units == "": // no units, use bytes
		current := units.HumanSize(float64(p.Current))
		if p.Total <= 0 || p.Total > p.Current {
			// remove total display if the reported current is wonky.
			return fmt.Sprintf("%8v", current)
		}
		total := units.HumanSize(float64(p.Total))
		return fmt.Sprintf("%8v/%v", current, total)
	default:
		if p.Total <= 0 || p.Total > p.Current {
			// remove total display if the reported current is wonky.
			return fmt.Sprintf("%d %s", p.Current, p.Units)
		}
		return fmt.Sprintf("%d/%d %s", p.Current, p.Total, p.Units)
	}
}
