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

package local

import (
	"context"
	"path"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/buildx/build"
	_ "github.com/docker/buildx/driver/docker" // required to get default driver registered
)

func (s *local) buildImage(ctx context.Context, service types.ServiceConfig, contextPath string) build.Options {
	var tags []string
	if service.Image != "" {
		tags = append(tags, service.Image)
	}

	if service.Build.Dockerfile == "" {
		service.Build.Dockerfile = "Dockerfile"
	}
	var buildArgs map[string]string

	return build.Options{
		Inputs: build.Inputs{
			ContextPath:    path.Join(contextPath, service.Build.Context),
			DockerfilePath: path.Join(contextPath, service.Build.Context, service.Build.Dockerfile),
		},
		BuildArgs: flatten(mergeArgs(service.Build.Args, buildArgs)),
		Tags:      tags,
	}
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
