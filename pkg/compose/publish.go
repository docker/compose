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
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// ociCompatibilityMode controls manifest generation to ensure compatibility
// with different registries.
//
// Currently, this is not exposed as an option to the user – Compose uses
// OCI 1.0 mode automatically for ECR registries based on domain and OCI 1.1
// for all other registries.
//
// There are likely other popular registries that do not support the OCI 1.1
// format, so it might make sense to expose this as a CLI flag or see if
// there's a way to generically probe the registry for support level.
type ociCompatibilityMode string

const (
	ociCompatibility1_0 ociCompatibilityMode = "1.0"
	ociCompatibility1_1 ociCompatibilityMode = "1.1"
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

	var layers []v1.Descriptor
	for _, file := range project.ComposeFiles {
		f, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		layer, err := s.pushComposeFile(ctx, file, f, resolver, named)
		if err != nil {
			return err
		}
		layers = append(layers, layer)
	}

	if options.ResolveImageDigests {
		yaml, err := s.generateImageDigestsOverride(ctx, project)
		if err != nil {
			return err
		}

		layer, err := s.pushComposeFile(ctx, "image-digests.yaml", yaml, resolver, named)
		if err != nil {
			return err
		}
		layers = append(layers, layer)
	}

	ociCompat := inferOCIVersion(named)
	toPush, err := s.generateManifest(layers, ociCompat)
	if err != nil {
		return err
	}

	w := progress.ContextWriter(ctx)
	w.Event(progress.Event{
		ID:     repository,
		Text:   "publishing",
		Status: progress.Working,
	})
	if !s.dryRun {
		for _, p := range toPush {
			err = resolver.Push(ctx, named, p.Descriptor, p.Data)
			if err != nil {
				return err
			}
		}
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

type push struct {
	Descriptor v1.Descriptor
	Data       []byte
}

func (s *composeService) generateManifest(layers []v1.Descriptor, ociCompat ociCompatibilityMode) ([]push, error) {
	var toPush []push
	var config v1.Descriptor
	var artifactType string
	switch ociCompat {
	case ociCompatibility1_0:
		configData, err := json.Marshal(v1.ImageConfig{})
		if err != nil {
			return nil, err
		}
		config = v1.Descriptor{
			MediaType: v1.MediaTypeImageConfig,
			Digest:    digest.FromBytes(configData),
			Size:      int64(len(configData)),
		}
		// N.B. OCI 1.0 does NOT support specifying the artifact type, so it's
		//		left as an empty string to omit it from the marshaled JSON
		artifactType = ""
		toPush = append(toPush, push{Descriptor: config, Data: configData})
	case ociCompatibility1_1:
		config = v1.DescriptorEmptyJSON
		artifactType = "application/vnd.docker.compose.project"
		// N.B. the descriptor has the data embedded in it
		toPush = append(toPush, push{Descriptor: config, Data: nil})
	default:
		return nil, fmt.Errorf("unsupported OCI version: %s", ociCompat)
	}

	manifest, err := json.Marshal(v1.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    v1.MediaTypeImageManifest,
		ArtifactType: artifactType,
		Config:       config,
		Layers:       layers,
		Annotations: map[string]string{
			"org.opencontainers.image.created": time.Now().Format(time.RFC3339),
		},
	})
	if err != nil {
		return nil, err
	}

	manifestDescriptor := v1.Descriptor{
		MediaType: v1.MediaTypeImageManifest,
		Digest:    digest.FromString(string(manifest)),
		Size:      int64(len(manifest)),
		Annotations: map[string]string{
			"com.docker.compose.version": api.ComposeVersion,
		},
		ArtifactType: artifactType,
	}
	toPush = append(toPush, push{Descriptor: manifestDescriptor, Data: manifest})
	return toPush, nil
}

func (s *composeService) generateImageDigestsOverride(ctx context.Context, project *types.Project) ([]byte, error) {
	project.ApplyProfiles([]string{"*"})
	err := project.ResolveImages(func(named reference.Named) (digest.Digest, error) {
		auth, err := encodedAuth(named, s.configFile())
		if err != nil {
			return "", err
		}
		inspect, err := s.apiClient().DistributionInspect(ctx, named.String(), auth)
		if err != nil {
			return "", err
		}
		return inspect.Descriptor.Digest, nil
	})
	if err != nil {
		return nil, err
	}
	override := types.Project{}
	for _, service := range project.Services {
		override.Services = append(override.Services, types.ServiceConfig{
			Name:  service.Name,
			Image: service.Image,
		})
	}
	return override.MarshalYAML()
}

func (s *composeService) pushComposeFile(ctx context.Context, file string, content []byte, resolver *imagetools.Resolver, named reference.Named) (v1.Descriptor, error) {
	w := progress.ContextWriter(ctx)
	w.Event(progress.Event{
		ID:     file,
		Text:   "publishing",
		Status: progress.Working,
	})
	layer := v1.Descriptor{
		MediaType: "application/vnd.docker.compose.file+yaml",
		Digest:    digest.FromString(string(content)),
		Size:      int64(len(content)),
		Annotations: map[string]string{
			"com.docker.compose.version": api.ComposeVersion,
			"com.docker.compose.file":    filepath.Base(file),
		},
	}
	err := resolver.Push(ctx, named, layer, content)
	w.Event(progress.Event{
		ID:     file,
		Text:   "published",
		Status: statusFor(err),
	})
	return layer, err
}

func statusFor(err error) progress.EventStatus {
	if err != nil {
		return progress.Error
	}
	return progress.Done
}

// inferOCIVersion uses OCI 1.1 by default but falls back to OCI 1.0 if the
// registry domain is known to require it.
//
// This is not ideal - with private registries, there isn't a bounded set of
// domains. As it stands, it's primarily intended for compatibility with AWS
// Elastic Container Registry (ECR) due to its ubiquity.
func inferOCIVersion(named reference.Named) ociCompatibilityMode {
	domain := reference.Domain(named)
	if strings.HasSuffix(domain, "amazonaws.com") {
		return ociCompatibility1_0
	} else {
		return ociCompatibility1_1
	}
}
