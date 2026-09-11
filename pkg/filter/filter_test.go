/*
   Copyright 2026 Docker Compose CLI authors

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

package filter

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func TestParse(t *testing.T) {
	tests := []struct {
		expressions []string
		expected    Filter
		expectedErr string
	}{
		{
			expressions: nil,
			expected:    nil,
		},
		{
			expressions: []string{"profile=foo"},
			expected:    Filter{{Criteria: "profile", Value: "foo"}},
		},
		{
			expressions: []string{"profile=foo", "label=tier=backend"},
			expected: Filter{
				{Criteria: "profile", Value: "foo"},
				{Criteria: "label", Value: "tier=backend"},
			},
		},
		{
			expressions: []string{"profile"},
			expectedErr: `invalid filter "profile": must be a criteria=value expression`,
		},
		{
			expressions: []string{"profile="},
			expectedErr: `invalid filter "profile=": must be a criteria=value expression`,
		},
		{
			expressions: []string{"state=running"},
			expectedErr: `invalid filter "state=running": unknown criteria "state" (supported: profile, label)`,
		},
		{
			expressions: []string{"profile=*"},
			expectedErr: `invalid filter "profile=*": profiles must be selected explicitly`,
		},
	}
	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			f, err := Parse(tc.expressions)
			if tc.expectedErr != "" {
				assert.Error(t, err, tc.expectedErr)
				return
			}
			assert.NilError(t, err)
			assert.Check(t, is.DeepEqual(f, tc.expected))
		})
	}
}

func TestProfiles(t *testing.T) {
	f, err := Parse([]string{"profile=foo", "profile=bar", "profile=foo", "label=x"})
	assert.NilError(t, err)
	assert.Check(t, is.DeepEqual(f.Profiles(), []string{"foo", "bar"}))
}

func TestSelectNames(t *testing.T) {
	project := &types.Project{
		Services: types.Services{
			"default": {Name: "default"},
			"labeled": {Name: "labeled", Labels: types.Labels{"tier": "backend"}},
			"foo1":    {Name: "foo1", Profiles: []string{"foo"}},
			"foo2":    {Name: "foo2", Profiles: []string{"foo", "bar"}, Labels: types.Labels{"tier": "backend"}},
			"bar1":    {Name: "bar1", Profiles: []string{"bar"}},
		},
	}

	tests := []struct {
		name        string
		expressions []string
		expected    []string
	}{
		{
			name:        "single profile",
			expressions: []string{"profile=foo"},
			expected:    []string{"foo1", "foo2"},
		},
		{
			name:        "profiles are alternatives",
			expressions: []string{"profile=foo", "profile=bar"},
			expected:    []string{"bar1", "foo1", "foo2"},
		},
		{
			name:        "label key",
			expressions: []string{"label=tier"},
			expected:    []string{"foo2", "labeled"},
		},
		{
			name:        "label key value",
			expressions: []string{"label=tier=backend"},
			expected:    []string{"foo2", "labeled"},
		},
		{
			name:        "label wrong value",
			expressions: []string{"label=tier=frontend"},
			expected:    nil,
		},
		{
			name:        "criteria combine",
			expressions: []string{"profile=foo", "label=tier=backend"},
			expected:    []string{"foo2"},
		},
		{
			name:        "no match",
			expressions: []string{"profile=unknown"},
			expected:    nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.expressions)
			assert.NilError(t, err)
			assert.Check(t, is.DeepEqual(f.SelectNames(project), tc.expected))
		})
	}
}
