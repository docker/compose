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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/docker/compose/v2/pkg/api"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/config/configfile"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/sanathkr/go-yaml"
)

// Separator is used for naming components
var Separator = "-"

// NewComposeService create a local implementation of the compose.Service API
func NewComposeService(apiClient client.APIClient, configFile *configfile.ConfigFile) api.Service {
	return &composeService{
		apiClient:  apiClient,
		configFile: configFile,
	}
}

type composeService struct {
	apiClient  client.APIClient
	configFile *configfile.ConfigFile
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
	project := c.Labels[api.ProjectLabel]
	prefix := fmt.Sprintf("%s_%s_", project, c.Labels[api.ServiceLabel])
	if strings.HasPrefix(name, prefix) {
		return name[len(project)+1:]
	}
	return name
}

func (s *composeService) Convert(ctx context.Context, project *types.Project, options api.ConvertOptions) ([]byte, error) {
	switch options.Format {
	case "json":
		marshal, err := json.MarshalIndent(project, "", "  ")
		if err != nil {
			return nil, err
		}
		return escapeDollarSign(marshal), nil
	case "yaml":
		marshal, err := yaml.Marshal(project)
		if err != nil {
			return nil, err
		}
		return escapeDollarSign(marshal), nil
	default:
		return nil, fmt.Errorf("unsupported format %q", options)
	}
}

func escapeDollarSign(marshal []byte) []byte {
	dollar := []byte{'$'}
	escDollar := []byte{'$', '$'}
	return bytes.ReplaceAll(marshal, dollar, escDollar)
}
