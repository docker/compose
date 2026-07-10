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
	"os"
	"path/filepath"
	"sort"
	"testing"

	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

const configFilterCompose = `
name: demo
services:
  web:
    image: nginx
    depends_on:
      - db
  api:
    image: api
    depends_on:
      db:
        condition: service_healthy
  db:
    image: postgres
  lonely:
    image: busybox
`

// modelServiceNames returns the sorted service names present in a raw compose model.
func modelServiceNames(t *testing.T, model map[string]any) []string {
	t.Helper()
	services, ok := model["services"].(map[string]any)
	assert.Assert(t, ok, "model has no services mapping")
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestToModelFiltersSelectedServices is a regression test for
// https://github.com/docker/compose/issues/13614: the [SERVICE...] argument was
// ignored on the `config --no-interpolate` / `config --variables` (raw model)
// code path, so every service was rendered regardless of the selection.
func TestToModelFiltersSelectedServices(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cli := mocks.NewMockCli(ctrl)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	assert.NilError(t, os.WriteFile(composePath, []byte(configFilterCompose), 0o600))

	for name, noNormalize := range map[string]bool{"normalized": false, "no-normalize": true} {
		t.Run(name, func(t *testing.T) {
			newOpts := func() *configOptions {
				return &configOptions{
					noInterpolate: true,
					noNormalize:   noNormalize,
					ProjectOptions: &ProjectOptions{
						ConfigPaths: []string{composePath},
						ProjectDir:  dir,
					},
				}
			}

			// A single selected service pulls in its (transitive) dependencies.
			model, err := newOpts().ToModel(t.Context(), cli, []string{"web"})
			assert.NilError(t, err)
			assert.DeepEqual(t, modelServiceNames(t, model), []string{"db", "web"})

			// A service declaring its dependency in long form is handled too.
			model, err = newOpts().ToModel(t.Context(), cli, []string{"api"})
			assert.NilError(t, err)
			assert.DeepEqual(t, modelServiceNames(t, model), []string{"api", "db"})

			// A standalone service is rendered on its own.
			model, err = newOpts().ToModel(t.Context(), cli, []string{"lonely"})
			assert.NilError(t, err)
			assert.DeepEqual(t, modelServiceNames(t, model), []string{"lonely"})

			// Multiple selected services are unioned with their dependencies.
			model, err = newOpts().ToModel(t.Context(), cli, []string{"web", "lonely"})
			assert.NilError(t, err)
			assert.DeepEqual(t, modelServiceNames(t, model), []string{"db", "lonely", "web"})

			// No selection renders the whole model.
			model, err = newOpts().ToModel(t.Context(), cli, nil)
			assert.NilError(t, err)
			assert.DeepEqual(t, modelServiceNames(t, model), []string{"api", "db", "lonely", "web"})

			// An unknown service is rejected like the fully typed path does.
			_, err = newOpts().ToModel(t.Context(), cli, []string{"nope"})
			assert.Error(t, err, "no such service: nope")
		})
	}
}

func TestFilterModelServices(t *testing.T) {
	baseModel := func() map[string]any {
		return map[string]any{
			"services": map[string]any{
				// short (list) depends_on form
				"web": map[string]any{"depends_on": []any{"db"}},
				// long (map) depends_on form, transitively reaching db
				"api": map[string]any{"depends_on": map[string]any{
					"cache": map[string]any{"condition": "service_started", "required": true},
				}},
				"cache":  map[string]any{"depends_on": []any{"db"}},
				"db":     map[string]any{},
				"lonely": map[string]any{},
			},
		}
	}

	names := func(model map[string]any) []string {
		out := []string{}
		for name := range model["services"].(map[string]any) {
			out = append(out, name)
		}
		sort.Strings(out)
		return out
	}

	t.Run("no selection is a no-op", func(t *testing.T) {
		model := baseModel()
		assert.NilError(t, filterModelServices(model, nil))
		assert.DeepEqual(t, names(model), []string{"api", "cache", "db", "lonely", "web"})
	})

	t.Run("transitive dependencies via list and map forms", func(t *testing.T) {
		model := baseModel()
		assert.NilError(t, filterModelServices(model, []string{"api"}))
		assert.DeepEqual(t, names(model), []string{"api", "cache", "db"})
	})

	t.Run("selection with a single dependency", func(t *testing.T) {
		model := baseModel()
		assert.NilError(t, filterModelServices(model, []string{"web"}))
		assert.DeepEqual(t, names(model), []string{"db", "web"})
	})

	t.Run("unknown service errors", func(t *testing.T) {
		model := baseModel()
		assert.Error(t, filterModelServices(model, []string{"missing"}), "no such service: missing")
	})

	t.Run("no services mapping errors when a service is requested", func(t *testing.T) {
		assert.Error(t, filterModelServices(map[string]any{}, []string{"web"}), "no such service: web")
	})
}
