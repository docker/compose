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
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/google/go-cmp/cmp"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/mock/gomock"
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
				Name: "web", ContainerSpec: types.ContainerSpec{
					Image: "nginx",
					EnvFiles: []types.EnvFile{
						{Path: envPath, Required: true},
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
	}

	accept, err := svc.preChecks(t.Context(), project, api.PublishOptions{})
	assert.NilError(t, err)
	assert.Equal(t, accept, false)
}

func Test_processFile_optional_env_file_missing(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	composeContent := `name: test
services:
  web:
    image: nginx
    env_file:
      - path: missing.env
        required: false
`
	assert.NilError(t, os.WriteFile(composePath, []byte(composeContent), 0o600))

	project, err := loader.LoadWithContext(t.Context(), types.ConfigDetails{
		WorkingDir:  dir,
		Environment: types.Mapping{},
		ConfigFiles: []types.ConfigFile{{Filename: composePath}},
	})
	assert.NilError(t, err)

	extFiles := map[string]string{}
	envFiles := map[string]string{}
	data, err := processFile(t.Context(), composePath, project, extFiles, envFiles)
	assert.NilError(t, err, "optional missing env file should not cause error")

	// The file is absent so nothing is registered for upload, but its path is still
	// rewritten to the opaque <hash>.env placeholder so the published artifact never leaks
	// the publisher's local path and stays consistent with the file-present case. The hash
	// derives from the path string alone, so it is deterministic regardless of existence.
	assert.Equal(t, len(envFiles), 0, "missing optional env file is not registered for upload")
	envPath := project.Services["web"].EnvFiles[0].Path
	hash := fmt.Sprintf("%x.env", sha256.Sum256([]byte(envPath)))
	assert.Assert(t, strings.Contains(string(data), hash), "published YAML should reference the hash placeholder")
	assert.Assert(t, !strings.Contains(string(data), "path: missing.env"), "published YAML must not leak the local path")
	assert.Assert(t, strings.Contains(string(data), "required: false"), "optional flag must be preserved")
}

func Test_processFile_optional_env_file_present(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "app.env")
	assert.NilError(t, os.WriteFile(envPath, []byte("FOO=bar\n"), 0o600))

	composePath := filepath.Join(dir, "compose.yaml")
	composeContent := `name: test
services:
  web:
    image: nginx
    env_file:
      - path: app.env
        required: false
`
	assert.NilError(t, os.WriteFile(composePath, []byte(composeContent), 0o600))

	project, err := loader.LoadWithContext(t.Context(), types.ConfigDetails{
		WorkingDir:  dir,
		Environment: types.Mapping{},
		ConfigFiles: []types.ConfigFile{{Filename: composePath}},
	})
	assert.NilError(t, err)

	extFiles := map[string]string{}
	envFiles := map[string]string{}
	_, err = processFile(t.Context(), composePath, project, extFiles, envFiles)
	assert.NilError(t, err)
	assert.Equal(t, len(envFiles), 1, "present optional env file should be added")
}

func Test_checkForSensitiveData_optional_env_file_missing(t *testing.T) {
	dir := t.TempDir()
	project := &types.Project{
		Services: types.Services{
			"web": {
				Name: "web", ContainerSpec: types.ContainerSpec{
					Image: "nginx",
					EnvFiles: []types.EnvFile{
						{Path: filepath.Join(dir, "missing.env"), Required: false},
					},
				},
			},
		},
	}

	svc := &composeService{}
	findings, err := svc.checkForSensitiveData(t.Context(), project)
	assert.NilError(t, err, "optional missing env file should not cause error during scan")
	assert.Equal(t, len(findings), 0)
}

func Test_checkForSensitiveData_optional_env_file_present(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "secrets.env")
	assert.NilError(t, os.WriteFile(envPath, []byte(`AWS_SECRET_ACCESS_KEY="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"`), 0o600))

	project := &types.Project{
		Services: types.Services{
			"web": {
				Name: "web", ContainerSpec: types.ContainerSpec{
					Image: "nginx",
					EnvFiles: []types.EnvFile{
						{Path: envPath, Required: false},
					},
				},
			},
		},
	}

	svc := &composeService{}
	findings, err := svc.checkForSensitiveData(t.Context(), project)
	assert.NilError(t, err)
	assert.Assert(t, len(findings) > 0, "present optional env file should still be scanned for secrets")
}

func Test_checkForSensitiveData_required_env_file_missing(t *testing.T) {
	dir := t.TempDir()
	project := &types.Project{
		Services: types.Services{
			"web": {
				Name: "web", ContainerSpec: types.ContainerSpec{
					Image: "nginx",
					EnvFiles: []types.EnvFile{
						{Path: filepath.Join(dir, "missing.env"), Required: true},
					},
				},
			},
		},
	}

	svc := &composeService{}
	_, err := svc.checkForSensitiveData(t.Context(), project)
	assert.ErrorContains(t, err, "not found", "required missing env file should fail")
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
		{
			// Jobs carry environment/env_file like services: same leak
			// surface, so they must be scanned too, not just project.Services.
			name: "job literal on suspicious key is flagged like a service's",
			files: map[string]string{
				"compose.yaml": `name: test
services:
  web:
    image: alpine
jobs:
  migrate:
    image: alpine
    environment:
      DB_PASSWORD: toto
    env_file:
      - ./app.env
    triggers:
      manual: true
`,
				"app.env": "FOO=bar\n",
			},
			wantSuspicious: map[string][]string{
				"migrate": {"DB_PASSWORD"},
			},
			wantEnvFile: []string{"migrate"},
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
	assert.Assert(t, strings.Contains(prompt.prompts[0], `"db"`))
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
				Name: "web", ContainerSpec: types.ContainerSpec{
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

func Test_generateImageDigestsOverride_resolvesDependentImages(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, cli := prepareMocks(mockCtrl)
	cli.EXPECT().ConfigFile().Return(configfile.New("")).AnyTimes()
	tested := &composeService{dockerCli: cli}

	// distinct digests per resolved reference, so attaching a digest to the wrong image would fail
	const (
		serviceDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		hookDigest    = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		volumeDigest  = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	)
	project := &types.Project{
		Name: "test",
		Services: types.Services{
			"app": types.ServiceConfig{
				Name: "app",

				PreStart: []types.PreStartHook{{ContainerSpec: types.ContainerSpec{Image: "hookimage:latest"}}}, ContainerSpec: types.ContainerSpec{
					Image: "nginx:latest",

					Volumes: []types.ServiceVolumeConfig{
						{Type: types.VolumeTypeImage, Source: "someimage:latest", Target: "/data"},
					},
				},
			},
		},
	}

	// compose-go resolves dependent images (pre_start hooks, type: image volume sources)
	// in WithImagesResolved, so publish now resolves them too: expect one registry call
	// per dependent image
	apiClient.EXPECT().DistributionInspect(gomock.Any(), "docker.io/library/nginx:latest", gomock.Any()).
		Return(client.DistributionInspectResult{
			DistributionInspect: registry.DistributionInspect{Descriptor: v1.Descriptor{Digest: serviceDigest}},
		}, nil)
	apiClient.EXPECT().DistributionInspect(gomock.Any(), "docker.io/library/hookimage:latest", gomock.Any()).
		Return(client.DistributionInspectResult{
			DistributionInspect: registry.DistributionInspect{Descriptor: v1.Descriptor{Digest: hookDigest}},
		}, nil)
	apiClient.EXPECT().DistributionInspect(gomock.Any(), "docker.io/library/someimage:latest", gomock.Any()).
		Return(client.DistributionInspectResult{
			DistributionInspect: registry.DistributionInspect{Descriptor: v1.Descriptor{Digest: volumeDigest}},
		}, nil)

	override, err := tested.generateImageDigestsOverride(t.Context(), project)
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(override), "docker.io/library/nginx:latest@"+serviceDigest))
}

// generateImageDigestsOverride only walks project.Services natively: jobs
// must be dressed as services to run through the exact same
// WithImagesResolved resolution, then folded back — so the published
// artifact is reproducible for jobs too, not just services.
func Test_generateImageDigestsOverride_resolvesJobImages(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, cli := prepareMocks(mockCtrl)
	cli.EXPECT().ConfigFile().Return(configfile.New("")).AnyTimes()
	tested := &composeService{dockerCli: cli}

	const jobDigest = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	project := &types.Project{
		Name: "test",
		Jobs: types.Jobs{
			"migrate": types.JobConfig{
				Name:          "migrate",
				ContainerSpec: types.ContainerSpec{Image: "migrate:latest"},
			},
		},
	}

	apiClient.EXPECT().DistributionInspect(gomock.Any(), "docker.io/library/migrate:latest", gomock.Any()).
		Return(client.DistributionInspectResult{
			DistributionInspect: registry.DistributionInspect{Descriptor: v1.Descriptor{Digest: jobDigest}},
		}, nil)

	override, err := tested.generateImageDigestsOverride(t.Context(), project)
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(override), "docker.io/library/migrate:latest@"+jobDigest))
}

// checkOnlyBuildSection must reject a job that only has a build section, the
// same as a build-only service: neither can be published as-is.
func Test_checkOnlyBuildSection_rejectsJobWithoutImage(t *testing.T) {
	project := &types.Project{
		Jobs: types.Jobs{
			"migrate": types.JobConfig{
				Name:         "migrate",
				WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{Context: "."}},
			},
		},
	}

	svc := &composeService{}
	ok, err := svc.checkOnlyBuildSection(project)
	assert.Assert(t, !ok)
	assert.ErrorContains(t, err, `"migrate"`)
}

func Test_checkOnlyBuildSection_acceptsJobWithImage(t *testing.T) {
	project := &types.Project{
		Jobs: types.Jobs{
			"migrate": types.JobConfig{
				Name:          "migrate",
				ContainerSpec: types.ContainerSpec{Image: "migrate:latest"},
				WorkloadSpec:  types.WorkloadSpec{Build: &types.BuildConfig{Context: "."}},
			},
		},
	}

	svc := &composeService{}
	ok, err := svc.checkOnlyBuildSection(project)
	assert.NilError(t, err)
	assert.Assert(t, ok)
}

// checkForBindMount must flag a job's bind mounts the same as a service's:
// a bind mount references the local filesystem, meaningless once published.
func Test_checkForBindMount_flagsJobBindMount(t *testing.T) {
	project := &types.Project{
		Jobs: types.Jobs{
			"migrate": types.JobConfig{
				Name: "migrate",
				ContainerSpec: types.ContainerSpec{
					Image: "migrate:latest",
					Volumes: []types.ServiceVolumeConfig{
						{Type: types.VolumeTypeBind, Source: "/host/data", Target: "/data"},
						{Type: types.VolumeTypeVolume, Source: "named", Target: "/named"},
					},
				},
			},
		},
	}

	svc := &composeService{}
	findings := svc.checkForBindMount(project)
	assert.Equal(t, len(findings["migrate"]), 1)
	assert.Equal(t, findings["migrate"][0].Source, "/host/data")
}

// checkForSensitiveData must scan a job's env files, same as a service's.
func Test_checkForSensitiveData_jobEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "secrets.env")
	assert.NilError(t, os.WriteFile(envPath, []byte(`AWS_SECRET_ACCESS_KEY="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"`), 0o600))

	project := &types.Project{
		Jobs: types.Jobs{
			"migrate": types.JobConfig{
				Name: "migrate",
				ContainerSpec: types.ContainerSpec{
					Image:    "migrate:latest",
					EnvFiles: []types.EnvFile{{Path: envPath, Required: false}},
				},
			},
		},
	}

	svc := &composeService{}
	findings, err := svc.checkForSensitiveData(t.Context(), project)
	assert.NilError(t, err)
	assert.Assert(t, len(findings) > 0, "job env file should be scanned for secrets like a service's")
}

// pushApplicationIndex walks every service AND job image to reference them
// in the application index manifest — a job's image must be part of the
// published artifact just like a service's. Copying/pushing a real image
// needs a live registry, out of reach for a unit test, but each image is
// parsed as a docker reference before that: an invalid job image surfaces
// its own parse error, proving the job loop is reached (a service-only walk
// would report no error at all, or a different one from the valid service).
func Test_pushApplicationIndex_walksJobImages(t *testing.T) {
	// No services at all: if the job loop didn't feed into the same walk,
	// there would be nothing to process and this would return nil, not the
	// job's own reference error.
	project := &types.Project{
		Jobs: types.Jobs{
			"migrate": types.JobConfig{Name: "migrate", ContainerSpec: types.ContainerSpec{Image: "Invalid/Image:Name"}},
		},
	}

	named, err := reference.ParseNormalizedNamed("myorg/myapp:latest")
	assert.NilError(t, err)

	err = pushApplicationIndex(t.Context(), nil, named, v1.Descriptor{}, project)
	assert.ErrorContains(t, err, "must be lowercase")
}
