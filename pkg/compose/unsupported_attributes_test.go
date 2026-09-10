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
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/compose-spec/compose-go/v2/schema"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// findingKey is the subset of api.UnsupportedAttribute we compare against a
// fixed expectation: Reason wording isn't part of the contract, only that a
// finding was raised for a given Service/Path pair (and that it carries a
// non-empty Reason, checked separately).
type findingKey struct {
	Service string
	Path    string
}

func findingKeys(t *testing.T, findings []api.UnsupportedAttribute) []findingKey {
	t.Helper()
	keys := make([]findingKey, 0, len(findings))
	for _, f := range findings {
		assert.Assert(t, f.Reason != "", "finding %+v is missing a Reason", f)
		keys = append(keys, findingKey{Service: f.Service, Path: f.Path})
	}
	return keys
}

// loadWithFiles writes files (name -> content) into a fresh temp dir and
// loads "compose.yaml" from it through the real compose-go loader with
// unsupported-attribute detection enabled, returning every finding.
// Detection is now inherently a load-time hook (compose-go's own
// tree-walker over the raw, un-normalized model): there is no longer a
// pure function to call directly on a hand-built types.Project, so every
// case here goes through an actual load.
func loadWithFiles(t *testing.T, files map[string]string) []api.UnsupportedAttribute {
	t.Helper()
	tmpDir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		assert.NilError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		assert.NilError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	service, err := NewComposeService(nil)
	assert.NilError(t, err)

	var findings []api.UnsupportedAttribute
	_, err = service.LoadProject(t.Context(), api.ProjectLoadOptions{
		ConfigPaths: []string{filepath.Join(tmpDir, "compose.yaml")},
		OnUnsupportedAttribute: func(f []api.UnsupportedAttribute) {
			findings = f
		},
	})
	assert.NilError(t, err)
	return findings
}

func loadCompose(t *testing.T, yaml string) []api.UnsupportedAttribute {
	t.Helper()
	return loadWithFiles(t, map[string]string{"compose.yaml": yaml})
}

func assertFindings(t *testing.T, got []api.UnsupportedAttribute, expected []findingKey) {
	t.Helper()
	slices.SortFunc(expected, func(a, b findingKey) int {
		if c := cmp.Compare(a.Service, b.Service); c != 0 {
			return c
		}
		return cmp.Compare(a.Path, b.Path)
	})
	assert.DeepEqual(t, findingKeys(t, got), expected, cmpopts.EquateEmpty())
}

func TestUnsupportedAttributes(t *testing.T) {
	t.Run("deploy mode is unsupported", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    deploy:
      mode: replicated
`)
		assertFindings(t, got, []findingKey{{Service: "web", Path: "deploy.mode"}})
	})

	t.Run("deploy labels is unsupported", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    deploy:
      labels:
        team: backend
`)
		assertFindings(t, got, []findingKey{{Service: "web", Path: "deploy.labels"}})
	})

	t.Run("deploy update_config is unsupported", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    deploy:
      update_config:
        parallelism: 1
`)
		assertFindings(t, got, []findingKey{{Service: "web", Path: "deploy.update_config"}})
	})

	t.Run("deploy rollback_config is unsupported", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    deploy:
      rollback_config:
        parallelism: 1
`)
		assertFindings(t, got, []findingKey{{Service: "web", Path: "deploy.rollback_config"}})
	})

	// Placement covers three independent sub-fields (constraints,
	// preferences, max_replicas_per_node). A single finding is reported for
	// the whole "deploy.placement" node regardless of which sub-field(s)
	// triggered it, rather than one finding per sub-field: they're only
	// ever set together in practice and placement has no further nesting
	// worth distinguishing in the report.
	t.Run("deploy placement constraints is unsupported", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    deploy:
      placement:
        constraints:
          - node.role==manager
`)
		assertFindings(t, got, []findingKey{{Service: "web", Path: "deploy.placement"}})
	})

	t.Run("deploy placement preferences is unsupported", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    deploy:
      placement:
        preferences:
          - spread: node.labels.zone
`)
		assertFindings(t, got, []findingKey{{Service: "web", Path: "deploy.placement"}})
	})

	t.Run("deploy placement max_replicas_per_node is unsupported", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    deploy:
      placement:
        max_replicas_per_node: 2
`)
		assertFindings(t, got, []findingKey{{Service: "web", Path: "deploy.placement"}})
	})

	t.Run("deploy endpoint_mode is unsupported", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    deploy:
      endpoint_mode: vip
`)
		assertFindings(t, got, []findingKey{{Service: "web", Path: "deploy.endpoint_mode"}})
	})

	t.Run("credential_spec is unsupported", func(t *testing.T) {
		got := loadWithFiles(t, map[string]string{
			"compose.yaml": `
services:
  web:
    image: alpine
    credential_spec:
      file: ./creds.json
`,
			"creds.json": "{}",
		})
		assertFindings(t, got, []findingKey{{Service: "web", Path: "credential_spec"}})
	})

	// label_file is NOT in presenceUnsupportedAttributes: compose-go's
	// loader resolves it into the service's actual Labels by default
	// (Options.SkipResolveLabels, which docker/compose never sets) — it's
	// fully functional here, unlike the genuinely Swarm-only attributes
	// above. This guards against it being re-added by mistake.
	t.Run("label_file is honored, no finding", func(t *testing.T) {
		tmpDir := t.TempDir()
		assert.NilError(t, os.WriteFile(filepath.Join(tmpDir, "labels.env"), []byte("team=backend\n"), 0o644))
		content := `
services:
  web:
    image: alpine
    label_file:
      - ./labels.env
`
		assert.NilError(t, os.WriteFile(filepath.Join(tmpDir, "compose.yaml"), []byte(content), 0o644))

		service, err := NewComposeService(nil)
		assert.NilError(t, err)
		var got []api.UnsupportedAttribute
		project, err := service.LoadProject(t.Context(), api.ProjectLoadOptions{
			ConfigPaths:            []string{filepath.Join(tmpDir, "compose.yaml")},
			OnUnsupportedAttribute: func(f []api.UnsupportedAttribute) { got = f },
		})
		assert.NilError(t, err)
		assertFindings(t, got, nil)
		assert.Equal(t, project.Services["web"].Labels["team"], "backend")
	})

	t.Run("port mode host is unsupported", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    ports:
      - target: 80
        published: "8080"
        protocol: tcp
        mode: host
`)
		assertFindings(t, got, []findingKey{{Service: "web", Path: "ports[80/tcp].mode"}})
	})

	// Each host-mode port is identified by its target port AND protocol: the
	// same target port can be published for both tcp and udp, and the two
	// must stay distinguishable instead of collapsing into identical lines.
	t.Run("same target port on different protocols is reported separately", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    ports:
      - target: 80
        published: "8080"
        protocol: tcp
        mode: host
      - target: 80
        published: "8080"
        protocol: udp
        mode: host
`)
		assertFindings(t, got, []findingKey{
			{Service: "web", Path: "ports[80/tcp].mode"},
			{Service: "web", Path: "ports[80/udp].mode"},
		})
	})

	// compose-go's loader defaults an unset ports[].mode to "ingress" for
	// every port entry, so a plain published port must NOT produce a
	// finding.
	t.Run("port mode ingress (the loader default) produces no finding", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    ports:
      - "8080:80"
`)
		assertFindings(t, got, nil)
	})

	t.Run("volume type cluster is unsupported", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    volumes:
      - type: cluster
        source: csi-vol
        target: /data
`)
		assertFindings(t, got, []findingKey{{Service: "web", Path: "volumes[csi-vol].type"}})
	})

	// Each cluster volume is identified by its source, so multiple such
	// volumes on the same service stay distinguishable.
	t.Run("multiple cluster volumes on the same service are reported separately", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    volumes:
      - type: cluster
        source: csi-vol-1
        target: /data1
      - type: cluster
        source: csi-vol-2
        target: /data2
`)
		assertFindings(t, got, []findingKey{
			{Service: "web", Path: "volumes[csi-vol-1].type"},
			{Service: "web", Path: "volumes[csi-vol-2].type"},
		})
	})

	// A bind mount, the most common volume shape, must never be flagged:
	// compose-go's loader defaults an unset volumes[].type to "bind" or
	// "volume" for every entry, so checking for mere presence of "type"
	// would fire on virtually every service using volumes.
	t.Run("volume type bind (the loader default) produces no finding", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    volumes:
      - /host:/container
`)
		assertFindings(t, got, nil)
	})

	// Migrated from the ad-hoc warning previously logged in create.go: a
	// config file reference's uid/gid/mode are silently ignored outside
	// Swarm mode. Each non-zero sub-field is reported individually, and the
	// path names the source so multiple config references stay
	// distinguishable.
	t.Run("service config file reference uid gid mode are unsupported", func(t *testing.T) {
		got := loadWithFiles(t, map[string]string{
			"compose.yaml": `
services:
  web:
    image: alpine
    configs:
      - source: c1
        uid: "1000"
        gid: "1000"
        mode: 0o400
configs:
  c1:
    file: ./c1.txt
`,
			"c1.txt": "hello",
		})
		assertFindings(t, got, []findingKey{
			{Service: "web", Path: "configs.c1.uid"},
			{Service: "web", Path: "configs.c1.gid"},
			{Service: "web", Path: "configs.c1.mode"},
		})
	})

	// Two config references with the same violation must stay
	// distinguishable by source name, not collapse into identical findings.
	t.Run("two config file references are reported separately by source", func(t *testing.T) {
		got := loadWithFiles(t, map[string]string{
			"compose.yaml": `
services:
  web:
    image: alpine
    configs:
      - source: c1
        uid: "1000"
      - source: c2
        uid: "1000"
configs:
  c1:
    file: ./c1.txt
  c2:
    file: ./c2.txt
`,
			"c1.txt": "hello",
			"c2.txt": "hello",
		})
		assertFindings(t, got, []findingKey{
			{Service: "web", Path: "configs.c1.uid"},
			{Service: "web", Path: "configs.c2.uid"},
		})
	})

	// Migrated from the ad-hoc warning previously logged in create.go: same
	// as above, for secret file references.
	t.Run("service secret file reference uid gid mode are unsupported", func(t *testing.T) {
		got := loadWithFiles(t, map[string]string{
			"compose.yaml": `
services:
  web:
    image: alpine
    secrets:
      - source: s1
        uid: "1000"
        gid: "1000"
        mode: 0o400
secrets:
  s1:
    file: ./s1.txt
`,
			"s1.txt": "hello",
		})
		assertFindings(t, got, []findingKey{
			{Service: "web", Path: "secrets.s1.uid"},
			{Service: "web", Path: "secrets.s1.gid"},
			{Service: "web", Path: "secrets.s1.mode"},
		})
	})

	// A config/secret reference with no uid/gid/mode override must never be
	// flagged: only the overrides are unsupported, not the reference itself.
	t.Run("config file reference without overrides produces no finding", func(t *testing.T) {
		got := loadWithFiles(t, map[string]string{
			"compose.yaml": `
services:
  web:
    image: alpine
    configs:
      - source: c1
configs:
  c1:
    file: ./c1.txt
`,
			"c1.txt": "hello",
		})
		assertFindings(t, got, nil)
	})

	// An unrecognized depends_on condition is not tested here: the
	// compose-spec schema declares it as a closed enum, so schema.Validate
	// rejects it before this check ever runs — see
	// TestDependsOnUnknownConditionIsRejectedBySchema below.

	t.Run("depends_on known condition produces no finding", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    depends_on:
      db:
        condition: service_healthy
  db:
    image: alpine
`)
		assertFindings(t, got, nil)
	})

	// The short-form depends_on (a plain list of service names) never
	// writes an explicit condition, so it must never be flagged either.
	t.Run("depends_on short form produces no finding", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    depends_on:
      - db
  db:
    image: alpine
`)
		assertFindings(t, got, nil)
	})

	t.Run("project-scoped config labels is unsupported", func(t *testing.T) {
		got := loadWithFiles(t, map[string]string{
			"compose.yaml": `
services:
  web:
    image: alpine
configs:
  c1:
    file: ./c1.txt
    labels:
      team: backend
`,
			"c1.txt": "hello",
		})
		assertFindings(t, got, []findingKey{{Service: "", Path: "configs.c1.labels"}})
	})

	// Unlike secrets, the compose-spec schema has no driver/driver_opts
	// property on a top-level configs entry (additionalProperties: false
	// rejects it at load time), so there is no equivalent case to test here
	// — a compose file setting it would fail to load entirely.

	t.Run("project-scoped secret driver_opts is unsupported", func(t *testing.T) {
		got := loadWithFiles(t, map[string]string{
			"compose.yaml": `
services:
  web:
    image: alpine
secrets:
  s1:
    file: ./s1.txt
    driver_opts:
      region: eu-west
`,
			"s1.txt": "hello",
		})
		assertFindings(t, got, []findingKey{{Service: "", Path: "secrets.s1.driver_opts"}})
	})

	t.Run("project-scoped secret labels is unsupported", func(t *testing.T) {
		got := loadWithFiles(t, map[string]string{
			"compose.yaml": `
services:
  web:
    image: alpine
secrets:
  s1:
    file: ./s1.txt
    labels:
      team: backend
`,
			"s1.txt": "hello",
		})
		assertFindings(t, got, []findingKey{{Service: "", Path: "secrets.s1.labels"}})
	})

	// --- False-positive traps: these MUST produce zero findings ---

	t.Run("deploy resources replicas and restart_policy are honored, no finding", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    deploy:
      replicas: 3
      resources:
        limits:
          cpus: "0.50"
          memory: 128M
      restart_policy:
        condition: on-failure
`)
		assertFindings(t, got, nil)
	})

	t.Run("baseline clean service produces no finding", func(t *testing.T) {
		got := loadCompose(t, `
services:
  web:
    image: alpine
    ports:
      - "8080:80"
    volumes:
      - /host:/container
`)
		assertFindings(t, got, nil)
	})

	t.Run("multiple findings on the same service are all reported", func(t *testing.T) {
		got := loadWithFiles(t, map[string]string{
			"compose.yaml": `
services:
  web:
    image: alpine
    deploy:
      mode: replicated
    credential_spec:
      file: ./creds.json
`,
			"creds.json": "{}",
		})
		assertFindings(t, got, []findingKey{
			{Service: "web", Path: "deploy.mode"},
			{Service: "web", Path: "credential_spec"},
		})
	})
}

// TestUnsupportedAttributes_SortedOutput asserts the reported findings are
// sorted: project services/configs/secrets are all Go maps, so without an
// explicit sort the order would vary between runs.
func TestUnsupportedAttributes_SortedOutput(t *testing.T) {
	// 6 distinctly-ordered names: with only 2-3 services, random map
	// iteration has a non-negligible chance of coincidentally coming out
	// sorted, masking a regression if the production sort were ever
	// removed.
	got := loadCompose(t, `
services:
  zeta:
    image: alpine
    deploy: {mode: replicated}
  echo:
    image: alpine
    deploy: {mode: replicated}
  delta:
    image: alpine
    deploy: {mode: replicated}
  charlie:
    image: alpine
    deploy: {mode: replicated}
  bravo:
    image: alpine
    deploy: {mode: replicated}
  alpha:
    image: alpine
    deploy: {mode: replicated}
`)
	assert.Assert(t, slices.IsSortedFunc(got, func(a, b api.UnsupportedAttribute) int {
		if c := cmp.Compare(a.Service, b.Service); c != 0 {
			return c
		}
		return cmp.Compare(a.Path, b.Path)
	}), "%+v", got)
}

// TestPresenceUnsupportedAttributes_AreKnownSchemaPaths guards
// presenceUnsupportedAttributes against typos: every key must be a real
// compose-spec path, or it silently does nothing (never removed from
// supportedAttributePaths, never matched against a real finding).
func TestPresenceUnsupportedAttributes_AreKnownSchemaPaths(t *testing.T) {
	all := schema.AttributePaths()
	for path := range presenceUnsupportedAttributes {
		assert.Assert(t, slices.Contains(all, path), "%q is not a known compose-spec attribute path", path)
	}
}

// TestDependsOnUnknownConditionIsRejectedBySchema documents why this
// package has no unsupported-attribute check for depends_on.*.condition:
// the compose-spec schema declares it as a closed enum of the 3 values this
// runtime understands, so an unrecognized value never reaches
// valueConditionalAttributes — it fails to load at all. waitDependency's
// own runtime fallback in service_containers.go covers the cases that
// bypass schema validation entirely (a project rebuilt from container
// labels, or one written by a different Compose version).
func TestDependsOnUnknownConditionIsRejectedBySchema(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	content := `
services:
  web:
    image: alpine
    depends_on:
      db:
        condition: service_ready
  db:
    image: alpine
`
	assert.NilError(t, os.WriteFile(composeFile, []byte(content), 0o644))

	service, err := NewComposeService(nil)
	assert.NilError(t, err)
	_, err = service.LoadProject(t.Context(), api.ProjectLoadOptions{ConfigPaths: []string{composeFile}})
	assert.ErrorContains(t, err, "condition")
}
