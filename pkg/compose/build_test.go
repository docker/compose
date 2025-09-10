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
	"slices"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func Test_addBuildDependencies(t *testing.T) {
	project := &types.Project{Services: types.Services{
		"test": types.ServiceConfig{
			Build: &types.BuildConfig{
				AdditionalContexts: map[string]string{
					"foo": "service:foo",
					"bar": "service:bar",
				},
			},
		},
		"foo": types.ServiceConfig{
			Build: &types.BuildConfig{
				AdditionalContexts: map[string]string{
					"zot": "service:zot",
				},
			},
		},
		"bar": types.ServiceConfig{
			Build: &types.BuildConfig{},
		},
		"zot": types.ServiceConfig{
			Build: &types.BuildConfig{},
		},
	}}

	services := addBuildDependencies([]string{"test"}, project)
	expected := []string{"test", "foo", "bar", "zot"}
	slices.Sort(services)
	slices.Sort(expected)
	assert.DeepEqual(t, services, expected)
}

func Test_toBakeAttest(t *testing.T) {
	tests := []struct {
		name     string
		build    types.BuildConfig
		expected []string
	}{
		{
			name:     "no attestations",
			build:    types.BuildConfig{},
			expected: nil,
		},
		{
			name:     "provenance",
			build:    types.BuildConfig{Provenance: "true"},
			expected: []string{"type=provenance"},
		},
		{
			name:     "max provenance",
			build:    types.BuildConfig{Provenance: "max"},
			expected: []string{"type=provenance,mode=max"},
		},
		{
			name:     "sbom",
			build:    types.BuildConfig{SBOM: "true"},
			expected: []string{"type=sbom"},
		},
		{
			name: "max provenance and sbom",
			build: types.BuildConfig{
				Provenance: "max",
				SBOM:       "true",
			},
			expected: []string{"type=provenance,mode=max", "type=sbom"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toBakeAttest(tt.build)
			assert.DeepEqual(t, result, tt.expected)
		})
	}
}
