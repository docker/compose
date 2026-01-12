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
	"fmt"
	"testing"

	"github.com/docker/docker/api/types/filters"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestGetDefaultFilters(t *testing.T) {
	tests := []struct {
		name             string
		projectName      string
		oneOff           oneOff
		selectedServices []string
		want             []filters.KeyValuePair
	}{
		{
			name:             "basic case with oneOffInclude and no services",
			projectName:      "myproject",
			oneOff:           oneOffInclude,
			selectedServices: []string{},
			want: []filters.KeyValuePair{
				filters.Arg("label", fmt.Sprintf("%s=%s", api.ProjectLabel, "myproject")),
				filters.Arg("label", api.ConfigHashLabel),
			},
		},
		{
			name:             "oneOffExclude adds oneOff false filter",
			projectName:      "myproject",
			oneOff:           oneOffExclude,
			selectedServices: []string{},
			want: []filters.KeyValuePair{
				filters.Arg("label", fmt.Sprintf("%s=%s", api.ProjectLabel, "myproject")),
				filters.Arg("label", api.ConfigHashLabel),
				filters.Arg("label", fmt.Sprintf("%s=%s", api.OneoffLabel, "False")),
			},
		},
		{
			name:             "oneOffOnly adds oneOff true filter",
			projectName:      "myproject",
			oneOff:           oneOffOnly,
			selectedServices: []string{},
			want: []filters.KeyValuePair{
				filters.Arg("label", fmt.Sprintf("%s=%s", api.ProjectLabel, "myproject")),
				filters.Arg("label", api.ConfigHashLabel),
				filters.Arg("label", fmt.Sprintf("%s=%s", api.OneoffLabel, "True")),
			},
		},
		{
			name:             "single selected service adds service filter",
			projectName:      "myproject",
			oneOff:           oneOffInclude,
			selectedServices: []string{"web"},
			want: []filters.KeyValuePair{
				filters.Arg("label", fmt.Sprintf("%s=%s", api.ProjectLabel, "myproject")),
				filters.Arg("label", fmt.Sprintf("%s=%s", api.ServiceLabel, "web")),
				filters.Arg("label", api.ConfigHashLabel),
			},
		},
		{
			name:             "multiple selected services do not add service filter",
			projectName:      "myproject",
			oneOff:           oneOffInclude,
			selectedServices: []string{"web", "db"},
			want: []filters.KeyValuePair{
				filters.Arg("label", fmt.Sprintf("%s=%s", api.ProjectLabel, "myproject")),
				filters.Arg("label", api.ConfigHashLabel),
			},
		},
		{
			name:             "combination: oneOffExclude with single service",
			projectName:      "testproject",
			oneOff:           oneOffExclude,
			selectedServices: []string{"api"},
			want: []filters.KeyValuePair{
				filters.Arg("label", fmt.Sprintf("%s=%s", api.ProjectLabel, "testproject")),
				filters.Arg("label", fmt.Sprintf("%s=%s", api.ServiceLabel, "api")),
				filters.Arg("label", api.ConfigHashLabel),
				filters.Arg("label", fmt.Sprintf("%s=%s", api.OneoffLabel, "False")),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDefaultFilters(tt.projectName, tt.oneOff, tt.selectedServices...)
			assert.DeepEqual(t, got, tt.want)
		})
	}
}
