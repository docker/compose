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
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/google/go-cmp/cmp"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal"
	"github.com/docker/compose/v5/pkg/api"
)

func Test_createLayers(t *testing.T) {
	project, err := loader.LoadWithContext(t.Context(), types.ConfigDetails{
		WorkingDir:  "testdata/publish/",
		Environment: types.Mapping{},
		ConfigFiles: []types.ConfigFile{
			{
				Filename: "testdata/publish/compose.yaml",
			},
		},
	})
	assert.NilError(t, err)
	project.ComposeFiles = []string{"testdata/publish/compose.yaml"}

	service := &composeService{}
	layers, err := service.createLayers(t.Context(), project, api.PublishOptions{
		WithEnvironment: true,
	})
	assert.NilError(t, err)

	published := string(layers[0].Data)
	assert.Equal(t, published, `name: test
services:
  test:
    extends:
      file: f8f9ede3d201ec37d5a5e3a77bbadab79af26035e53135e19571f50d541d390c.yaml
      service: foo

  string:
    image: test
    env_file: 5efca9cdbac9f5394c6c2e2094b1b42661f988f57fcab165a0bf72b205451af3.env

  list:
    image: test
    env_file:
      - 5efca9cdbac9f5394c6c2e2094b1b42661f988f57fcab165a0bf72b205451af3.env

  mapping:
    image: test
    env_file:
      - path: 5efca9cdbac9f5394c6c2e2094b1b42661f988f57fcab165a0bf72b205451af3.env
`)

	expectedLayers := []v1.Descriptor{
		{
			MediaType: "application/vnd.docker.compose.file+yaml",
			Annotations: map[string]string{
				"com.docker.compose.file":    "compose.yaml",
				"com.docker.compose.version": internal.Version,
			},
		},
		{
			MediaType: "application/vnd.docker.compose.file+yaml",
			Annotations: map[string]string{
				"com.docker.compose.extends": "true",
				"com.docker.compose.file":    "f8f9ede3d201ec37d5a5e3a77bbadab79af26035e53135e19571f50d541d390c",
				"com.docker.compose.version": internal.Version,
			},
		},
		{
			MediaType: "application/vnd.docker.compose.envfile",
			Annotations: map[string]string{
				"com.docker.compose.envfile": "5efca9cdbac9f5394c6c2e2094b1b42661f988f57fcab165a0bf72b205451af3",
				"com.docker.compose.version": internal.Version,
			},
		},
	}
	assert.DeepEqual(t, expectedLayers, layers, cmp.FilterPath(func(path cmp.Path) bool {
		return !slices.Contains([]string{".Data", ".Digest", ".Size"}, path.String())
	}, cmp.Ignore()))
}

func Test_preChecks_sensitive_data_detected_decline(t *testing.T) {
	dir := t.TempDir()
	envPath := dir + "/secrets.env"
	secretData := `AWS_SECRET_ACCESS_KEY="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"`
	err := os.WriteFile(envPath, []byte(secretData), 0o600)
	assert.NilError(t, err)

	project := &types.Project{
		Services: types.Services{
			"web": {
				Name:  "web",
				Image: "nginx",
				EnvFiles: []types.EnvFile{
					{Path: envPath, Required: true},
				},
			},
		},
	}

	declined := func(message string, defaultValue bool) (bool, error) {
		return false, nil
	}
	svc := &composeService{
		prompt: declined,
	}

	accept, err := svc.preChecks(t.Context(), project, api.PublishOptions{})
	assert.NilError(t, err)
	assert.Equal(t, accept, false)
}

// --- collectEnvCheckFindings: pure detection logic ---

func loadProjectForTest(t *testing.T, files map[string]string) *types.Project {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		assert.NilError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		assert.NilError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	composePath := filepath.Join(dir, "compose.yaml")
	project, err := loader.LoadWithContext(t.Context(), types.ConfigDetails{
		WorkingDir:  dir,
		Environment: types.Mapping{},
		ConfigFiles: []types.ConfigFile{{Filename: composePath}},
	}, func(options *loader.Options) {
		options.SetProjectName("test", true)
	})
	assert.NilError(t, err)
	project.ComposeFiles = []string{composePath}
	return project
}

func Test_collectEnvCheckFindings(t *testing.T) {
	tests := []struct {
		name            string
		files           map[string]string
		wantSuspicious  map[string][]string // service -> sorted suspicious keys
		wantEnvFile     []string            // services with env_file
		wantLiteralCfgs []string            // config names with literal content
	}{
		{
			name: "benign literals are silent",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  web:
    image: alpine
    environment:
      LOG_LEVEL: info
      NODE_ENV: production
      PORT: "8080"
`,
			},
		},
		{
			name: "interpolated values are silent even on suspicious keys",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  web:
    image: alpine
    environment:
      DB_PASSWORD: "${DB_PASSWORD}"
      API_KEY: "$API_KEY"
`,
			},
		},
		{
			name: "literal value on suspicious key is flagged",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  db:
    image: mysql
    environment:
      MYSQL_ROOT_PASSWORD: toto
      MYSQL_DATABASE: appdb
`,
			},
			wantSuspicious: map[string][]string{
				"db": {"MYSQL_ROOT_PASSWORD"},
			},
		},
		{
			name: "demo placeholder changeme is flagged (security: literal still leaks)",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  demo:
    image: postgres
    environment:
      DB_PASSWORD: changeme
`,
			},
			wantSuspicious: map[string][]string{
				"demo": {"DB_PASSWORD"},
			},
		},
		{
			name: "multiple suspicious keys on one service are aggregated and sorted",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  api:
    image: alpine
    environment:
      DB_PASSWORD: toto
      API_KEY: foo
      DEBUG: "1"
`,
			},
			// DEBUG is benign — only suspicious-named keys appear.
			wantSuspicious: map[string][]string{
				"api": {"API_KEY", "DB_PASSWORD"},
			},
		},
		{
			name: "nil-valued env (KEY without =) is silent",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  web:
    image: alpine
    environment:
      - PASSWORD
`,
			},
		},
		{
			name: "env_file declaration is reported separately",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  legacy:
    image: alpine
    env_file:
      - ./app.env
`,
				"app.env": "FOO=bar\n",
			},
			wantEnvFile: []string{"legacy"},
		},
		{
			name: "literal config.content is flagged",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  app:
    image: alpine
configs:
  cfg:
    content: |
      api_key=plaintext
`,
			},
			wantLiteralCfgs: []string{"cfg"},
		},
		{
			name: "interpolated config.content is silent",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  app:
    image: alpine
configs:
  cfg:
    content: "key=${SECRET}"
`,
			},
		},
		{
			name: "config.environment is silent (only the var name is published)",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  app:
    image: alpine
configs:
  cfg:
    environment: HARDCODED
`,
			},
		},
		{
			name: "compose-spec $$ escape on suspicious key is flagged (literal $ leaks)",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  db:
    image: mysql
    environment:
      MYSQL_ROOT_PASSWORD: "$$literal"
`,
			},
			wantSuspicious: map[string][]string{
				"db": {"MYSQL_ROOT_PASSWORD"},
			},
		},
		{
			name: "embedded $$ in middle of value is flagged (pa$$word resolves to pa$word)",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  db:
    image: mysql
    environment:
      DB_PASSWORD: "pa$$word"
`,
			},
			wantSuspicious: map[string][]string{
				"db": {"DB_PASSWORD"},
			},
		},
		{
			name: "single $ remains interpolation (escape fix does not regress this)",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  web:
    image: alpine
    environment:
      DB_PASSWORD: "$VAR"
      API_KEY: "${TOKEN}"
`,
			},
		},
		{
			name: "extends walks parent file and reports inherited literals",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  api:
    extends:
      file: ./base.yaml
      service: api-base
`,
				"base.yaml": `services:
  api-base:
    image: alpine
    environment:
      INHERITED_PASSWORD: toto
`,
			},
			// The extends walk surfaces parent-file findings under the parent's
			// service name, since that's what gets serialized into the OCI artifact.
			wantSuspicious: map[string][]string{
				"api-base": {"INHERITED_PASSWORD"},
			},
		},
		{
			name: "extends parent unrelated services are also reported (they leak too)",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  api:
    extends:
      file: ./base.yaml
      service: api-base
`,
				"base.yaml": `services:
  api-base:
    image: alpine
    environment:
      INHERITED_PASSWORD: shared-toto
  unrelated:
    image: alpine
    environment:
      UNRELATED_SECRET: lonely-toto
`,
			},
			wantSuspicious: map[string][]string{
				"api-base":  {"INHERITED_PASSWORD"},
				"unrelated": {"UNRELATED_SECRET"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := loadProjectForTest(t, tt.files)

			findings, err := collectEnvCheckFindings(t.Context(), project)
			assert.NilError(t, err)

			gotSuspicious := map[string][]string{}
			var gotEnvFile []string
			for name, f := range findings.services {
				if keys := f.sortedSuspiciousKeys(); len(keys) > 0 {
					gotSuspicious[name] = keys
				}
				if f.hasEnvFile {
					gotEnvFile = append(gotEnvFile, name)
				}
			}
			slices.Sort(gotEnvFile)

			if tt.wantSuspicious == nil {
				tt.wantSuspicious = map[string][]string{}
			}
			assert.DeepEqual(t, tt.wantSuspicious, gotSuspicious)
			assert.DeepEqual(t, tt.wantEnvFile, gotEnvFile)
			assert.DeepEqual(t, tt.wantLiteralCfgs, findings.configsLiteralContent)
		})
	}
}

// --- checkEnvironmentVariables: prompt orchestration ---

type fakePrompt struct {
	answers []bool   // queued answers; consumed FIFO
	prompts []string // captured prompt messages
}

func (p *fakePrompt) handler(message string, _ bool) (bool, error) {
	p.prompts = append(p.prompts, message)
	if len(p.answers) == 0 {
		return true, nil
	}
	a := p.answers[0]
	p.answers = p.answers[1:]
	return a, nil
}

func Test_checkEnvironmentVariables_silent_when_no_findings(t *testing.T) {
	project := loadProjectForTest(t, map[string]string{
		"compose.yaml": `name: test
services:
  web:
    image: alpine
    environment:
      LOG_LEVEL: info
      DB_HOST: "${DATABASE_HOST}"
`,
	})

	prompt := &fakePrompt{}
	svc := &composeService{prompt: prompt.handler}

	err := svc.checkEnvironmentVariables(t.Context(), project, api.PublishOptions{})
	assert.NilError(t, err)
	assert.Equal(t, len(prompt.prompts), 0, "no prompt expected for benign config")
}

func Test_checkEnvironmentVariables_prompts_on_suspicious_literal(t *testing.T) {
	project := loadProjectForTest(t, map[string]string{
		"compose.yaml": `name: test
services:
  db:
    image: mysql
    environment:
      MYSQL_ROOT_PASSWORD: toto
`,
	})

	prompt := &fakePrompt{answers: []bool{true}}
	svc := &composeService{prompt: prompt.handler}

	err := svc.checkEnvironmentVariables(t.Context(), project, api.PublishOptions{})
	assert.NilError(t, err)
	assert.Equal(t, len(prompt.prompts), 1, "exactly one env-related prompt")
	assert.Assert(t, strings.Contains(prompt.prompts[0], `service "db"`))
	assert.Assert(t, strings.Contains(prompt.prompts[0], "MYSQL_ROOT_PASSWORD"))
}

func Test_checkEnvironmentVariables_decline_returns_ErrCanceled(t *testing.T) {
	project := loadProjectForTest(t, map[string]string{
		"compose.yaml": `name: test
services:
  db:
    image: mysql
    environment:
      MYSQL_ROOT_PASSWORD: toto
`,
	})

	prompt := &fakePrompt{answers: []bool{false}}
	svc := &composeService{prompt: prompt.handler}

	err := svc.checkEnvironmentVariables(t.Context(), project, api.PublishOptions{})
	assert.Assert(t, errors.Is(err, api.ErrCanceled),
		"decline should return api.ErrCanceled, got: %v", err)
}

func Test_checkEnvironmentVariables_with_env_silences_env_prompt(t *testing.T) {
	// --with-env should silence env_file + literal-env prompts, but config.content
	// has its own prompt path that runs regardless.
	project := loadProjectForTest(t, map[string]string{
		"compose.yaml": `name: test
services:
  db:
    image: mysql
    environment:
      MYSQL_ROOT_PASSWORD: toto
  legacy:
    image: alpine
    env_file:
      - ./app.env
configs:
  cfg:
    content: |
      api_key=plaintext
`,
		"app.env": "FOO=bar\n",
	})

	prompt := &fakePrompt{answers: []bool{true}}
	svc := &composeService{prompt: prompt.handler}

	err := svc.checkEnvironmentVariables(t.Context(), project, api.PublishOptions{WithEnvironment: true})
	assert.NilError(t, err)
	assert.Equal(t, len(prompt.prompts), 1, "only the config.content prompt should fire")
	assert.Assert(t, strings.Contains(prompt.prompts[0], `config "cfg"`))
}

func Test_checkEnvironmentVariables_two_prompts_when_env_and_config(t *testing.T) {
	project := loadProjectForTest(t, map[string]string{
		"compose.yaml": `name: test
services:
  db:
    image: mysql
    environment:
      MYSQL_ROOT_PASSWORD: toto
configs:
  cfg:
    content: |
      api_key=plaintext
`,
	})

	prompt := &fakePrompt{answers: []bool{true, true}}
	svc := &composeService{prompt: prompt.handler}

	err := svc.checkEnvironmentVariables(t.Context(), project, api.PublishOptions{})
	assert.NilError(t, err)
	assert.Equal(t, len(prompt.prompts), 2, "expected env prompt then config.content prompt")
}

func Test_publish_decline_returns_ErrCanceled(t *testing.T) {
	project := &types.Project{
		Services: types.Services{
			"web": {
				Name:  "web",
				Image: "nginx",
				Volumes: []types.ServiceVolumeConfig{
					{
						Type:   types.VolumeTypeBind,
						Source: "/host/path",
						Target: "/container/path",
					},
				},
			},
		},
	}

	declined := func(message string, defaultValue bool) (bool, error) {
		return false, nil
	}
	svc := &composeService{
		prompt: declined,
		events: &ignore{},
	}

	err := svc.publish(t.Context(), project, "docker.io/myorg/myapp:latest", api.PublishOptions{})
	assert.Assert(t, errors.Is(err, api.ErrCanceled),
		"expected api.ErrCanceled when user declines, got: %v", err)
}
