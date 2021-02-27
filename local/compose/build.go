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
	"os"
	"path"
	"strings"

	"github.com/docker/compose-cli/api/compose"

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/containerd/platforms"
	"github.com/docker/buildx/build"
	"github.com/docker/buildx/driver"
	_ "github.com/docker/buildx/driver/docker" // required to get default driver registered
	"github.com/docker/buildx/util/progress"
	"github.com/docker/docker/errdefs"
	bclient "github.com/moby/buildkit/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

func (s *composeService) Build(ctx context.Context, project *types.Project, options compose.BuildOptions) error {
	opts := map[string]build.Options{}
	imagesToBuild := []string{}
	for _, service := range project.Services {
		if service.Build != nil {
			imageName := getImageName(service, project.Name)
			imagesToBuild = append(imagesToBuild, imageName)
			buildOptions, err := s.toBuildOptions(service, project.WorkingDir, imageName)
			if err != nil {
				return err
			}
			buildOptions.Pull = options.Pull
			buildOptions.BuildArgs = options.Args
			opts[imageName] = buildOptions
		}
	}

	err := s.build(ctx, project, opts, options.Progress)
	if err == nil {
		displayScanSuggestMsg(ctx, imagesToBuild)
	}

	return err
}

func (s *composeService) ensureImagesExists(ctx context.Context, project *types.Project) error {
	opts := map[string]build.Options{}
	imagesToBuild := []string{}
	for _, service := range project.Services {
		if service.Image == "" && service.Build == nil {
			return fmt.Errorf("invalid service %q. Must specify either image or build", service.Name)
		}

		imageName := getImageName(service, project.Name)
		localImagePresent, err := s.localImagePresent(ctx, imageName)
		if err != nil {
			return err
		}

		if service.Build != nil {
			if localImagePresent && service.PullPolicy != types.PullPolicyBuild {
				continue
			}
			imagesToBuild = append(imagesToBuild, imageName)
			opts[imageName], err = s.toBuildOptions(service, project.WorkingDir, imageName)
			if err != nil {
				return err
			}
			continue
		}
		if service.Image != "" {
			if localImagePresent {
				continue
			}
		}

		// Buildx has no command to "just pull", see
		// so we bake a temporary dockerfile that will just pull and export pulled image
		opts[service.Name] = build.Options{
			Inputs: build.Inputs{
				ContextPath:    ".",
				DockerfilePath: "-",
				InStream:       strings.NewReader("FROM " + service.Image),
			},
			Tags: []string{service.Image},
			Pull: true,
		}

	}

	err := s.build(ctx, project, opts, "auto")
	if err == nil {
		displayScanSuggestMsg(ctx, imagesToBuild)
	}
	return err
}

func (s *composeService) localImagePresent(ctx context.Context, imageName string) (bool, error) {
	_, _, err := s.apiClient.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *composeService) build(ctx context.Context, project *types.Project, opts map[string]build.Options, mode string) error {
	if len(opts) == 0 {
		return nil
	}
	const drivername = "default"
	d, err := driver.GetDriver(ctx, drivername, nil, s.apiClient, nil, nil, nil, "", nil, nil, project.WorkingDir)
	if err != nil {
		return err
	}
	driverInfo := []build.DriverInfo{
		{
			Name:   "default",
			Driver: d,
		},
	}

	// Progress needs its own context that lives longer than the
	// build one otherwise it won't read all the messages from
	// build and will lock
	progressCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := progress.NewPrinter(progressCtx, os.Stdout, mode)

	// We rely on buildx "docker" builder integrated in docker engine, so don't need a DockerAPI here
	_, err = build.Build(ctx, driverInfo, opts, nil, nil, w)
	errW := w.Wait()
	if err == nil {
		err = errW
	}
	return err
}

func (s *composeService) toBuildOptions(service types.ServiceConfig, contextPath string, imageTag string) (build.Options, error) {
	var tags []string
	tags = append(tags, imageTag)

	if service.Build.Dockerfile == "" {
		service.Build.Dockerfile = "Dockerfile"
	}
	var buildArgs map[string]string

	var plats []specs.Platform
	if service.Platform != "" {
		p, err := platforms.Parse(service.Platform)
		if err != nil {
			return build.Options{}, err
		}
		plats = append(plats, p)
	}

	return build.Options{
		Inputs: build.Inputs{
			ContextPath:    path.Join(contextPath, service.Build.Context),
			DockerfilePath: path.Join(contextPath, service.Build.Context, service.Build.Dockerfile),
		},
		BuildArgs: flatten(mergeArgs(service.Build.Args, buildArgs)),
		Tags:      tags,
		Target:    service.Build.Target,
		Exports:   []bclient.ExportEntry{{Type: "image", Attrs: map[string]string{}}},
		Platforms: plats,
	}, nil
}

func flatten(in types.MappingWithEquals) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string)
	for k, v := range in {
		if v == nil {
			continue
		}
		out[k] = *v
	}
	return out
}

func mergeArgs(src types.MappingWithEquals, values map[string]string) types.MappingWithEquals {
	for key := range src {
		if val, ok := values[key]; ok {
			if val == "" {
				src[key] = nil
			} else {
				src[key] = &val
			}
		}
	}
	return src
}
