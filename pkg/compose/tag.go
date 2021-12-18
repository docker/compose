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
	"bytes"
	"context"
	"fmt"
	"html/template"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose/v2/pkg/api"
	"golang.org/x/sync/errgroup"
)

type TagTemplate struct {
	ServiceName          string
	ProjectName          string
	ServicesCount        int
	ServicesToBuildCount int
}

func (s *composeService) Tag(ctx context.Context, project *types.Project, options api.TagOptions) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, service := range project.Services {
		if service.Build == nil {
			continue
		}
		service := service
		eg.Go(func() error {
			currentImage := getImageName(service, project.Name)
			newImageTag := fmt.Sprintf("%s/%s", project.Name, service.Name)
			if options.Template != "" {
				newImageTag = interpolateString(options.Template, TagTemplate{
					ServiceName:          service.Name,
					ProjectName:          project.Name,
					ServicesCount:        len(project.Services),
					ServicesToBuildCount: countServicesToBuild(project.Services),
				})
			}
			fmt.Printf("Tagging %s to %s\n", currentImage, newImageTag)
			err := s.apiClient.ImageTag(ctx, currentImage, newImageTag)
			if err != nil {
				return err
			}
			// pushing image if requested
			if options.Push {
				service.Image = newImageTag
				projectCopy := project
				projectCopy.Services = []types.ServiceConfig{service}
				errPush := s.Push(ctx, projectCopy, api.PushOptions{
					IgnoreFailures: options.IgnorePushFailures,
				})
				if errPush != nil {
					return errPush
				}
			}
			return nil
		})
	}
	return eg.Wait()
}

func interpolate(t *template.Template, vars interface{}) string {
	var tmplBytes bytes.Buffer

	err := t.Execute(&tmplBytes, vars)
	if err != nil {
		panic(err)
	}
	return tmplBytes.String()
}

func interpolateString(str string, vars interface{}) string {
	tmpl, err := template.New("tmpl").Parse(str)

	if err != nil {
		panic(err)
	}
	return interpolate(tmpl, vars)
}

func countServicesToBuild(services types.Services) (c int) {
	c = 0
	for _, s := range services {
		if s.Build != nil {
			c++
		}
	}
	return c
}
