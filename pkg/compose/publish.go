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
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/distribution/distribution/v3/reference"
	client2 "github.com/docker/cli/cli/registry/client"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func (s *composeService) Publish(ctx context.Context, project *types.Project, repository string) error {
	err := s.Push(ctx, project, api.PushOptions{})
	if err != nil {
		return err
	}

	target, err := reference.ParseDockerRef(repository)
	if err != nil {
		return err
	}
	client := s.dockerCli.RegistryClient(false)
	for i, service := range project.Services {
		ref, err := reference.ParseDockerRef(service.Image)
		if err != nil {
			return err
		}
		auth, err := encodedAuth(ref, s.configFile())
		if err != nil {
			return err
		}
		inspect, err := s.apiClient().DistributionInspect(ctx, ref.String(), auth)
		if err != nil {
			return err
		}
		canonical, err := reference.WithDigest(ref, inspect.Descriptor.Digest)
		if err != nil {
			return err
		}
		to, err := reference.WithDigest(target, inspect.Descriptor.Digest)
		if err != nil {
			return err
		}
		err = client.MountBlob(ctx, canonical, to)
		switch err.(type) {
		case client2.ErrBlobCreated:
		default:
			return err
		}
		service.Image = to.String()
		project.Services[i] = service
	}

	err = s.publishComposeYaml(ctx, project, repository)
	if err != nil {
		return err
	}
	return nil
}

func (s *composeService) publishComposeYaml(ctx context.Context, project *types.Project, repository string) error {
	ref, err := reference.ParseDockerRef(repository)
	if err != nil {
		return err
	}

	var manifests []v1.Descriptor

	for _, composeFile := range project.ComposeFiles {
		stat, err := os.Stat(composeFile)
		if err != nil {
			return err
		}

		cmd := exec.CommandContext(ctx, "oras", "push", "--artifact-type", "application/vnd.docker.compose.yaml", ref.String(), composeFile)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		cmd.Stderr = s.stderr()

		err = cmd.Start()
		if err != nil {
			return err
		}
		out, err := io.ReadAll(stdout)
		if err != nil {
			return err
		}
		var composeFileDigest string
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "Digest: ") {
				composeFileDigest = line[len("Digest: "):]
			}
			fmt.Fprintln(s.stdout(), line)
		}
		if composeFileDigest == "" {
			return fmt.Errorf("expected oras to display `Digest: xxx`")
		}

		err = cmd.Wait()
		if err != nil {
			return err
		}

		manifests = append(manifests, v1.Descriptor{
			MediaType:    "application/vnd.oci.image.manifest.v1+json",
			Digest:       digest.Digest(composeFileDigest),
			Size:         stat.Size(),
			ArtifactType: "application/vnd.docker.compose.yaml",
		})
	}

	for _, service := range project.Services {
		dockerRef, err := reference.ParseDockerRef(service.Image)
		if err != nil {
			return err
		}
		manifests = append(manifests, v1.Descriptor{
			MediaType: v1.MediaTypeImageIndex,
			Digest:    dockerRef.(reference.Digested).Digest(),
			Annotations: map[string]string{
				"com.docker.compose.service": service.Name,
			},
		})
	}

	manifest := v1.Index{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType: v1.MediaTypeImageIndex,
		Manifests: manifests,
		Annotations: map[string]string{
			"com.docker.compose": api.ComposeVersion,
		},
	}
	manifestContent, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(os.TempDir(), "compose")
	if err != nil {
		return err
	}
	err = os.WriteFile(temp.Name(), manifestContent, 0o700)
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	cmd := exec.CommandContext(ctx, "oras", "manifest", "push", ref.String(), temp.Name())
	cmd.Stdout = s.stdout()
	cmd.Stderr = s.stderr()
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
