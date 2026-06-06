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

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestProjectHasRefs(t *testing.T) {
	str := func(s string) *string { return &s }

	tests := []struct {
		name     string
		project  *types.Project
		expected bool
	}{
		{
			name: "no refs",
			project: &types.Project{
				Services: types.Services{
					"web": {Environment: types.MappingWithEquals{"FOO": str("bar")}},
				},
			},
			expected: false,
		},
		{
			name: "has openbao ref",
			project: &types.Project{
				Services: types.Services{
					"web": {Environment: types.MappingWithEquals{
						"SECRET": str("ref+openbao://secret/data/prod/db#/password"),
					}},
				},
			},
			expected: true,
		},
		{
			name: "nil value ignored",
			project: &types.Project{
				Services: types.Services{
					"web": {Environment: types.MappingWithEquals{"FOO": nil}},
				},
			},
			expected: false,
		},
		{
			name:     "empty project",
			project:  &types.Project{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, projectHasRefs(tt.project), tt.expected)
		})
	}
}

func TestResolveSecretReferences_NoRefs(t *testing.T) {
	str := func(s string) *string { return &s }

	project := &types.Project{
		Services: types.Services{
			"web": {Environment: types.MappingWithEquals{"FOO": str("bar")}},
		},
	}

	err := resolveSecretReferences(project)
	assert.NilError(t, err)
	assert.Equal(t, *project.Services["web"].Environment["FOO"], "bar")
}

func TestResolveSecretReferences_UnsupportedScheme(t *testing.T) {
	str := func(s string) *string { return &s }

	project := &types.Project{
		Services: types.Services{
			"web": {Environment: types.MappingWithEquals{
				"SECRET": str("ref+unsupported://some/path#/key"),
			}},
		},
	}

	err := resolveSecretReferences(project)
	assert.ErrorContains(t, err, "unsupported secret reference scheme")
}

func TestResolveSecretReferences_WithMockResolver(t *testing.T) {
	str := func(s string) *string { return &s }

	// Register a mock resolver for testing
	original := resolverRegistry
	resolverRegistry = map[string]resolverFactory{
		"ref+mock://": func() (SecretResolver, error) {
			return &mockResolver{secrets: map[string]map[string]string{
				"secret/prod/db": {
					"username": "admin",
					"password": "s3cret",
				},
			}}, nil
		},
	}
	defer func() { resolverRegistry = original }()

	project := &types.Project{
		Services: types.Services{
			"web": {Environment: types.MappingWithEquals{
				"DB_USER": str("ref+mock://secret/prod/db#/username"),
				"DB_PASS": str("ref+mock://secret/prod/db#/password"),
				"STATIC":  str("plain-value"),
			}},
		},
	}

	err := resolveSecretReferences(project)
	assert.NilError(t, err)
	assert.Equal(t, *project.Services["web"].Environment["DB_USER"], "admin")
	assert.Equal(t, *project.Services["web"].Environment["DB_PASS"], "s3cret")
	assert.Equal(t, *project.Services["web"].Environment["STATIC"], "plain-value")
}

func TestResolveSecretReferences_ResolverError(t *testing.T) {
	str := func(s string) *string { return &s }

	original := resolverRegistry
	resolverRegistry = map[string]resolverFactory{
		"ref+mock://": func() (SecretResolver, error) {
			return &mockResolver{secrets: map[string]map[string]string{}}, nil
		},
	}
	defer func() { resolverRegistry = original }()

	project := &types.Project{
		Services: types.Services{
			"web": {Environment: types.MappingWithEquals{
				"SECRET": str("ref+mock://secret/missing/path#/key"),
			}},
		},
	}

	err := resolveSecretReferences(project)
	assert.ErrorContains(t, err, "no data at path")
}

func TestParseRef(t *testing.T) {
	tests := []struct {
		ref      string
		wantPath string
		wantKey  string
	}{
		{
			ref:      "ref+openbao://secret/g/team/app/prod/db#/password",
			wantPath: "secret/g/team/app/prod/db",
			wantKey:  "password",
		},
		{
			ref:      "ref+openbao://secret/prod/postgres#/username",
			wantPath: "secret/prod/postgres",
			wantKey:  "username",
		},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			path, key, err := parseRef(tt.ref)
			assert.NilError(t, err)
			assert.Equal(t, path, tt.wantPath)
			assert.Equal(t, key, tt.wantKey)
		})
	}
}

func TestParseRef_Invalid(t *testing.T) {
	_, _, err := parseRef("ref+openbao://secret/path/without/key")
	assert.ErrorContains(t, err, "missing #/key")
}

func TestInsertKVv2Data(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"secret/myapp/db", "secret/data/myapp/db"},
		{"secret/g/team/app/prod/db", "secret/data/g/team/app/prod/db"},
		{"kv/foo", "kv/data/foo"},
		{"kv/prod/api", "kv/data/prod/api"},
		{"secret/data/myapp/db", "secret/data/myapp/db"},
		{"kv/data/prod/api", "kv/data/prod/api"},
		{"secret/myapp/data/db", "secret/data/myapp/data/db"},
		{"single", "single"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, insertKVv2Data(tt.input), tt.expected)
		})
	}
}

// mockResolver implements SecretResolver for testing.
type mockResolver struct {
	secrets map[string]map[string]string
}

func (m *mockResolver) Resolve(path, key string) (string, error) {
	data, ok := m.secrets[path]
	if !ok {
		return "", fmt.Errorf("no data at path %q", path)
	}
	val, ok := data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found at path %q", key, path)
	}
	return val, nil
}
