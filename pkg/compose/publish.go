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
	"os"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/distribution/reference"
	"github.com/docker/buildx/util/imagetools"
	"github.com/docker/compose/v2/internal/ocipush"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
)

func (s *composeService) Publish(ctx context.Context, project *types.Project, repository string, options api.PublishOptions) error {
	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		return s.publish(ctx, project, repository, options)
	}, s.stdinfo(), "Publishing")
}

func (s *composeService) publish(ctx context.Context, project *types.Project, repository string, options api.PublishOptions) error {
	err := s.Push(ctx, project, api.PushOptions{})
	if err != nil {
		return err
	}

	named, err := reference.ParseDockerRef(repository)
	if err != nil {
		return err
	}

	resolver := imagetools.New(imagetools.Opt{
		Auth: s.configFile(),
	})

	var layers []ocipush.Pushable
	for _, file := range project.ComposeFiles {
		f, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		layerDescriptor := ocipush.DescriptorForComposeFile(file, f)
		layers = append(layers, ocipush.Pushable{
			Descriptor: layerDescriptor,
			Data:       f,
		})
	}

	if options.ResolveImageDigests {
		yaml, err := s.generateImageDigestsOverride(ctx, project)
		if err != nil {
			return err
		}

		layerDescriptor := ocipush.DescriptorForComposeFile("image-digests.yaml", yaml)
		layers = append(layers, ocipush.Pushable{
			Descriptor: layerDescriptor,
			Data:       yaml,
		})
	}

	w := progress.ContextWriter(ctx)
	w.Event(progress.Event{
		ID:     repository,
		Text:   "publishing",
		Status: progress.Working,
	})
	if !s.dryRun {
		err = ocipush.PushManifest(ctx, resolver, named, layers, options.OCIVersion)
		if err != nil {
			w.Event(progress.Event{
				ID:     repository,
				Text:   "publishing",
				Status: progress.Error,
			})
			return err
		}
	}
	w.Event(progress.Event{
		ID:     repository,
		Text:   "published",
		Status: progress.Done,
	})
	return nil
}

func (s *composeService) generateImageDigestsOverride(ctx context.Context, project *types.Project) ([]byte, error) {
	project, err := project.WithProfiles([]string{"*"})
	if err != nil {
		return nil, err
	}
	project, err = project.WithImagesResolved(ImageDigestResolver(ctx, s.configFile(), s.apiClient()))
	if err != nil {
		return nil, err
	}
	override := types.Project{
		Services: types.Services{},
	}
	for name, service := range project.Services {
		override.Services[name] = types.ServiceConfig{
			Image: service.Image,
		}
	}
	return override.MarshalYAML()
}
