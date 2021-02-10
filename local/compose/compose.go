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
	"strings"

	"github.com/docker/compose-cli/api/compose"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/sanathkr/go-yaml"

	errdefs2 "github.com/docker/compose-cli/api/errdefs"
)

// NewComposeService create a local implementation of the compose.Service API
func NewComposeService(apiClient client.APIClient) compose.Service {
	return &composeService{apiClient: apiClient}
}

type composeService struct {
	apiClient client.APIClient
}

func (s *composeService) Up(ctx context.Context, project *types.Project, options compose.UpOptions) error {
	return errdefs2.ErrNotImplemented
}

func getCanonicalContainerName(c moby.Container) string {
	// Names return container canonical name /foo  + link aliases /linked_by/foo
	for _, name := range c.Names {
		if strings.LastIndex(name, "/") == 0 {
			return name[1:]
		}
	}
	return c.Names[0][1:]
}

func getContainerNameWithoutProject(c moby.Container) string {
	name := getCanonicalContainerName(c)
	project := c.Labels[projectLabel]
	prefix := fmt.Sprintf("%s_%s_", project, c.Labels[serviceLabel])
	if strings.HasPrefix(name, prefix) {
		return name[len(project)+1:]
	}
	return name
}

func (s *composeService) Convert(ctx context.Context, project *types.Project, options compose.ConvertOptions) ([]byte, error) {
	switch options.Format {
	case "json":
		return json.MarshalIndent(project, "", "  ")
	case "yaml":
		return yaml.Marshal(project)
	default:
		return nil, fmt.Errorf("unsupported format %q", options)
	}
}
