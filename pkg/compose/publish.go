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
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/compose-spec/compose-go/types"
	"github.com/distribution/reference"
	"github.com/docker/buildx/util/imagetools"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
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

	w := progress.ContextWriter(ctx)

	named, err := reference.ParseDockerRef(repository)
	if err != nil {
		return err
	}

	resolver := imagetools.New(imagetools.Opt{
		Auth: s.configFile(),
	})

	var layers []v1.Descriptor
	for _, file := range project.ComposeFiles {
		f, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		w.Event(progress.Event{
			ID:     file,
			Text:   "publishing",
			Status: progress.Working,
		})
		layer := v1.Descriptor{
			MediaType: "application/vnd.docker.compose.file+yaml",
			Digest:    digest.FromString(string(f)),
			Size:      int64(len(f)),
			Annotations: map[string]string{
				"com.docker.compose.version": api.ComposeVersion,
				"com.docker.compose.file":    filepath.Base(file),
			},
		}
		layers = append(layers, layer)
		err = resolver.Push(ctx, named, layer, f)
		if err != nil {
			w.Event(progress.Event{
				ID:     file,
				Text:   "publishing",
				Status: progress.Error,
			})

			return err
		}

		w.Event(progress.Event{
			ID:     file,
			Text:   "published",
			Status: progress.Done,
		})
	}

	emptyConfig, err := json.Marshal(v1.ImageConfig{})
	if err != nil {
		return err
	}
	configDescriptor := v1.Descriptor{
		MediaType: "application/vnd.oci.empty.v1+json",
		Digest:    digest.FromBytes(emptyConfig),
		Size:      int64(len(emptyConfig)),
	}
	var imageManifest []byte
	if !s.dryRun {
		err = resolver.Push(ctx, named, configDescriptor, emptyConfig)
		if err != nil {
			return err
		}
		imageManifest, err = json.Marshal(v1.Manifest{
			Versioned:    specs.Versioned{SchemaVersion: 2},
			MediaType:    v1.MediaTypeImageManifest,
			ArtifactType: "application/vnd.docker.compose.project",
			Config:       configDescriptor,
			Layers:       layers,
			Annotations: map[string]string{
				"org.opencontainers.image.created": time.Now().Format(time.RFC3339),
			},
		})
		if err != nil {
			return err
		}
	}

	w.Event(progress.Event{
		ID:     repository,
		Text:   "publishing",
		Status: progress.Working,
	})
	if !s.dryRun {
		err = resolver.Push(ctx, named, v1.Descriptor{
			MediaType: v1.MediaTypeImageManifest,
			Digest:    digest.FromString(string(imageManifest)),
			Size:      int64(len(imageManifest)),
			Annotations: map[string]string{
				"com.docker.compose.version": api.ComposeVersion,
			},
			ArtifactType: "application/vnd.docker.compose.project",
		}, imageManifest)
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
