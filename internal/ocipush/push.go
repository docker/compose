/*
   Copyright 2023 Docker Compose CLI authors

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

package ocipush

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	pusherrors "github.com/containerd/containerd/remotes/errors"
	"github.com/distribution/reference"
	"github.com/docker/buildx/util/imagetools"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// ComposeProjectArtifactType is the OCI 1.1-compliant artifact type value
	// for the generated image manifest.
	ComposeProjectArtifactType = "application/vnd.docker.compose.project"
	// ComposeYAMLMediaType is the media type for each layer (Compose file)
	// in the image manifest.
	ComposeYAMLMediaType = "application/vnd.docker.compose.file+yaml"
	// ComposeEmptyConfigMediaType is a media type used for the config descriptor
	// when doing OCI 1.0-style pushes.
	//
	// The content is always `{}`, the same as a normal empty descriptor, but
	// the specific media type allows clients to fall back to the config media
	// type to recognize the manifest as a Compose project since the artifact
	// type field is not available in OCI 1.0.
	//
	// This is based on guidance from the OCI 1.1 spec:
	//	> Implementers note: artifacts have historically been created without
	// 	> an artifactType field, and tooling to work with artifacts should
	//	> fallback to the config.mediaType value.
	ComposeEmptyConfigMediaType = "application/vnd.docker.compose.config.empty.v1+json"
)

// clientAuthStatusCodes are client (4xx) errors that are authentication
// related.
var clientAuthStatusCodes = []int{
	http.StatusUnauthorized,
	http.StatusForbidden,
	http.StatusProxyAuthRequired,
}

type Pushable struct {
	Descriptor v1.Descriptor
	Data       []byte
}

func DescriptorForComposeFile(path string, content []byte) v1.Descriptor {
	return v1.Descriptor{
		MediaType: ComposeYAMLMediaType,
		Digest:    digest.FromString(string(content)),
		Size:      int64(len(content)),
		Annotations: map[string]string{
			"com.docker.compose.version": api.ComposeVersion,
			"com.docker.compose.file":    filepath.Base(path),
		},
	}
}

func PushManifest(
	ctx context.Context,
	resolver *imagetools.Resolver,
	named reference.Named,
	layers []Pushable,
	ociVersion api.OCIVersion,
) error {
	// Check if we need an extra empty layer for the manifest config
	if ociVersion == api.OCIVersion1_1 || ociVersion == "" {
		layers = append(layers, Pushable{Descriptor: v1.DescriptorEmptyJSON, Data: []byte("{}")})
	}
	// prepare to push the manifest by pushing the layers
	layerDescriptors := make([]v1.Descriptor, len(layers))
	for i := range layers {
		layerDescriptors[i] = layers[i].Descriptor
		if err := resolver.Push(ctx, named, layers[i].Descriptor, layers[i].Data); err != nil {
			return err
		}
	}

	if ociVersion != "" {
		// if a version was explicitly specified, use it
		return createAndPushManifest(ctx, resolver, named, layerDescriptors, ociVersion)
	}

	// try to push in the OCI 1.1 format but fallback to OCI 1.0 on 4xx errors
	// (other than auth) since it's most likely the result of the registry not
	// having support
	err := createAndPushManifest(ctx, resolver, named, layerDescriptors, api.OCIVersion1_1)
	var pushErr pusherrors.ErrUnexpectedStatus
	if errors.As(err, &pushErr) && isNonAuthClientError(pushErr.StatusCode) {
		// TODO(milas): show a warning here (won't work with logrus)
		return createAndPushManifest(ctx, resolver, named, layerDescriptors, api.OCIVersion1_0)
	}
	return err
}

func createAndPushManifest(
	ctx context.Context,
	resolver *imagetools.Resolver,
	named reference.Named,
	layers []v1.Descriptor,
	ociVersion api.OCIVersion,
) error {
	toPush, err := generateManifest(layers, ociVersion)
	if err != nil {
		return err
	}
	for _, p := range toPush {
		err = resolver.Push(ctx, named, p.Descriptor, p.Data)
		if err != nil {
			return err
		}
	}
	return nil
}

func isNonAuthClientError(statusCode int) bool {
	if statusCode < 400 || statusCode >= 500 {
		// not a client error
		return false
	}
	for _, v := range clientAuthStatusCodes {
		if statusCode == v {
			// client auth error
			return false
		}
	}
	// any other 4xx client error
	return true
}

func generateManifest(layers []v1.Descriptor, ociCompat api.OCIVersion) ([]Pushable, error) {
	var toPush []Pushable
	var config v1.Descriptor
	var artifactType string
	switch ociCompat {
	case api.OCIVersion1_0:
		// "Content other than OCI container images MAY be packaged using the image manifest.
		// When this is done, the config.mediaType value MUST be set to a value specific to
		// the artifact type or the empty value."
		// Source: https://github.com/opencontainers/image-spec/blob/main/manifest.md#guidelines-for-artifact-usage
		//
		// The `ComposeEmptyConfigMediaType` is used specifically for this purpose:
		// there is no config, and an empty descriptor is used for OCI 1.1 in
		// conjunction with the `ArtifactType`, but for OCI 1.0 compatibility,
		// tooling falls back to the config media type, so this is used to
		// indicate that it's not a container image but custom content.
		configData := []byte("{}")
		config = v1.Descriptor{
			MediaType: ComposeEmptyConfigMediaType,
			Digest:    digest.FromBytes(configData),
			Size:      int64(len(configData)),
		}
		// N.B. OCI 1.0 does NOT support specifying the artifact type, so it's
		//		left as an empty string to omit it from the marshaled JSON
		artifactType = ""
		toPush = append(toPush, Pushable{Descriptor: config, Data: configData})
	case api.OCIVersion1_1:
		config = v1.DescriptorEmptyJSON
		artifactType = ComposeProjectArtifactType
		// N.B. the descriptor has the data embedded in it
		toPush = append(toPush, Pushable{Descriptor: config, Data: make([]byte, len(config.Data))})
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
	toPush = append(toPush, Pushable{Descriptor: manifestDescriptor, Data: manifest})
	return toPush, nil
}
