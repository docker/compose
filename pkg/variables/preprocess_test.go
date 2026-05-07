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

package variables

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	full := filepath.Join(dir, name)
	assert.NilError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	assert.NilError(t, os.WriteFile(full, []byte(body), 0o644))
	return full
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	assert.NilError(t, err)
	return string(b)
}

func noShell(string) (string, bool) { return "", false }

func TestRenderInlineVariables(t *testing.T) {
	dir := t.TempDir()
	root := writeFile(t, dir, "compose.yaml", `variables:
  APP_VERSION: "1.4.2"
services:
  api:
    image: ghcr.io/acme/api:${APP_VERSION}
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rendered := readFile(t, r.ConfigPaths[0])
	assert.Assert(t, !contains(rendered, "variables:"))
	assert.Assert(t, contains(rendered, "ghcr.io/acme/api:1.4.2"))
}

func TestRenderVariablesFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "vars/base.yaml", `variables:
  APP_VERSION: "1.0.0"
  REDIS_VERSION: "7.0"
`)
	writeFile(t, dir, "vars/dev.yaml", `variables:
  APP_VERSION: "1.4.2"
`)
	root := writeFile(t, dir, "compose.yaml", `variables_file:
  - ./vars/base.yaml
  - ./vars/dev.yaml
services:
  api:
    image: api:${APP_VERSION}
    environment:
      REDIS: redis:${REDIS_VERSION}
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rendered := readFile(t, r.ConfigPaths[0])
	assert.Assert(t, contains(rendered, "api:1.4.2"))
	assert.Assert(t, contains(rendered, "redis:7.0"))
}

func TestRenderInlineOverridesVariablesFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "vars.yaml", `variables:
  APP_VERSION: "from-file"
`)
	root := writeFile(t, dir, "compose.yaml", `variables:
  APP_VERSION: "from-inline"
variables_file:
  - ./vars.yaml
services:
  api:
    image: api:${APP_VERSION}
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rendered := readFile(t, r.ConfigPaths[0])
	assert.Assert(t, contains(rendered, "api:from-inline"))
}

func TestRenderVariablesFileScalarForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "vars.yaml", `variables:
  APP_VERSION: "1.4.2"
`)
	root := writeFile(t, dir, "compose.yaml", `variables_file: ./vars.yaml
services:
  api:
    image: api:${APP_VERSION}
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rendered := readFile(t, r.ConfigPaths[0])
	assert.Assert(t, contains(rendered, "api:1.4.2"))
}

func TestRenderStripsTopLevelVariablesFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "vars.yaml", `variables:
  APP_VERSION: "1.4.2"
`)
	root := writeFile(t, dir, "compose.yaml", `variables_file:
  - ./vars.yaml
services:
  api:
    image: api:${APP_VERSION}
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rendered := readFile(t, r.ConfigPaths[0])
	// compose-go would reject unknown top-level keys; verify
	// `variables_file:` is stripped from the rendered output.
	assert.Assert(t, !contains(rendered, "variables_file"))
}

func TestRenderCLIVarOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	root := writeFile(t, dir, "compose.yaml", `variables:
  APP_VERSION: "yaml"
services:
  api:
    image: api:${APP_VERSION}
`)

	r, err := Render(t.Context(), []string{root}, []string{"APP_VERSION=cli"}, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rendered := readFile(t, r.ConfigPaths[0])
	assert.Assert(t, contains(rendered, "api:cli"))
}

func TestRenderShellOverridesCLI(t *testing.T) {
	dir := t.TempDir()
	root := writeFile(t, dir, "compose.yaml", `variables:
  APP_VERSION: "yaml"
services:
  api:
    image: api:${APP_VERSION}
`)
	shell := func(name string) (string, bool) {
		if name == "APP_VERSION" {
			return "shellv", true
		}
		return "", false
	}

	r, err := Render(t.Context(), []string{root}, []string{"APP_VERSION=cli"}, nil, shell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rendered := readFile(t, r.ConfigPaths[0])
	assert.Assert(t, contains(rendered, "api:shellv"))
}

func TestRenderVarFile(t *testing.T) {
	dir := t.TempDir()
	varFile := writeFile(t, dir, "override.yaml", `variables:
  APP_VERSION: "from-var-file"
`)
	root := writeFile(t, dir, "compose.yaml", `variables:
  APP_VERSION: "yaml"
services:
  api:
    image: api:${APP_VERSION}
`)

	r, err := Render(t.Context(), []string{root}, nil, []string{varFile}, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rendered := readFile(t, r.ConfigPaths[0])
	assert.Assert(t, contains(rendered, "api:from-var-file"))
}

func TestRenderCrossRef(t *testing.T) {
	dir := t.TempDir()
	root := writeFile(t, dir, "compose.yaml", `variables:
  BASE: "acme"
  IMAGE: "${BASE}/api"
  TAG: "1.4.2"
  FULL: "${IMAGE}:${TAG}"
services:
  api:
    image: ${FULL}
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rendered := readFile(t, r.ConfigPaths[0])
	assert.Assert(t, contains(rendered, "image: acme/api:1.4.2"))
}

func TestRenderCrossRefCycleErrors(t *testing.T) {
	dir := t.TempDir()
	root := writeFile(t, dir, "compose.yaml", `variables:
  A: "${B}"
  B: "${A}"
services:
  api:
    image: ${A}
`)

	_, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.ErrorContains(t, err, "cyclic")
}

func TestRenderIncludeLocalOverridesRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "redis/compose.yaml", `services:
  redis:
    image: redis:${REDIS_VERSION}
`)
	root := writeFile(t, dir, "compose.yaml", `variables:
  REDIS_VERSION: "7.2"
include:
  - path: ./redis/compose.yaml
    variables:
      REDIS_VERSION: "7.4"
services: {}
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	// Find the rendered redis file by walking the tempdir.
	var redisRendered string
	_ = filepath.Walk(filepath.Dir(r.ConfigPaths[0]), func(path string, _ os.FileInfo, _ error) error {
		if filepath.Base(path) == "compose.yaml" && contains(path, "redis") {
			redisRendered = path
		}
		return nil
	})
	assert.Assert(t, redisRendered != "")
	body := readFile(t, redisRendered)
	assert.Assert(t, contains(body, "redis:7.4"))
}

func TestRenderNoLeakageBetweenSiblings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "redis/compose.yaml", `services:
  redis:
    image: redis:${MODULE}
`)
	writeFile(t, dir, "postgres/compose.yaml", `services:
  postgres:
    image: postgres:${MODULE}
`)
	root := writeFile(t, dir, "compose.yaml", `include:
  - path: ./redis/compose.yaml
    variables:
      MODULE: "redis-only"
  - path: ./postgres/compose.yaml
services: {}
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	// Postgres's MODULE must NOT pick up redis's include-local var.
	var redisFile, pgFile string
	_ = filepath.Walk(filepath.Dir(r.ConfigPaths[0]), func(path string, _ os.FileInfo, _ error) error {
		switch {
		case contains(path, "/redis/") && filepath.Base(path) == "compose.yaml":
			redisFile = path
		case contains(path, "/postgres/") && filepath.Base(path) == "compose.yaml":
			pgFile = path
		}
		return nil
	})
	assert.Assert(t, redisFile != "" && pgFile != "")
	assert.Assert(t, contains(readFile(t, redisFile), "redis-only"))
	assert.Assert(t, !contains(readFile(t, pgFile), "redis-only"))
}

func TestRenderIncludedFileTopLevelScopedToItself(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "redis/compose.yaml", `variables:
  REDIS_LOCAL: "internal"
services:
  redis:
    image: redis:${REDIS_LOCAL}
`)
	root := writeFile(t, dir, "compose.yaml", `include:
  - path: ./redis/compose.yaml
services:
  api:
    image: api:${REDIS_LOCAL}
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rootBody := readFile(t, r.ConfigPaths[0])
	// REDIS_LOCAL undeclared at root → empty.
	assert.Assert(t, !contains(rootBody, "internal"))
	assert.Assert(t, !contains(rootBody, "REDIS_LOCAL"))

	// Redis include sees its own top-level variable.
	var redisFile string
	_ = filepath.Walk(filepath.Dir(r.ConfigPaths[0]), func(path string, _ os.FileInfo, _ error) error {
		if contains(path, "/redis/") && filepath.Base(path) == "compose.yaml" {
			redisFile = path
		}
		return nil
	})
	assert.Assert(t, redisFile != "")
	assert.Assert(t, contains(readFile(t, redisFile), "redis:internal"))
}

func TestRenderCoerceTypes(t *testing.T) {
	dir := t.TempDir()
	root := writeFile(t, dir, "compose.yaml", `variables:
  PORT: 8080
  ENABLED: true
services:
  api:
    image: api
    ports:
      - "${PORT}:80"
    environment:
      ENABLED: "${ENABLED}"
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	body := readFile(t, r.ConfigPaths[0])
	assert.Assert(t, contains(body, "8080:80"))
	assert.Assert(t, contains(body, "ENABLED: \"true\"") || contains(body, "ENABLED: 'true'") || contains(body, "ENABLED: true"))
}

func TestRenderStripsExtensionKeysFromIncludes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "redis/compose.yaml", `services:
  redis:
    image: redis:${REDIS_VERSION}
`)
	root := writeFile(t, dir, "compose.yaml", `include:
  - path: ./redis/compose.yaml
    variables:
      REDIS_VERSION: "7.4"
services: {}
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rendered := readFile(t, r.ConfigPaths[0])
	// `variables:` must be stripped from the include entry so
	// compose-go's strict schema doesn't reject it.
	assert.Assert(t, !contains(rendered, "REDIS_VERSION"))
}

func TestRenderUndeclaredWarnsAndEmpties(t *testing.T) {
	dir := t.TempDir()
	root := writeFile(t, dir, "compose.yaml", `services:
  api:
    image: api:${NEVER_DECLARED}
`)

	r, err := Render(t.Context(), []string{root}, nil, nil, noShell)
	assert.NilError(t, err)
	defer r.Cleanup()

	rendered := readFile(t, r.ConfigPaths[0])
	assert.Assert(t, !contains(rendered, "NEVER_DECLARED"))
	assert.Assert(t, !contains(rendered, "${"))
}

func contains(s, sub string) bool {
	return indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	if sub == "" {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
