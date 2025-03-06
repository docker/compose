/*
   Copyright 2023 Docker Compose CLI authors

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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/pkg/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestApplyPlatforms_InferFromRuntime(t *testing.T) {
	makeProject := func() *types.Project {
		return &types.Project{
			Services: types.Services{
				"test": {
					Name:  "test",
					Image: "foo",
					Build: &types.BuildConfig{
						Context: ".",
						Platforms: []string{
							"linux/amd64",
							"linux/arm64",
							"alice/32",
						},
					},
					Platform: "alice/32",
				},
			},
		}
	}

	t.Run("SinglePlatform", func(t *testing.T) {
		project := makeProject()
		require.NoError(t, applyPlatforms(project, true))
		require.EqualValues(t, []string{"alice/32"}, project.Services["test"].Build.Platforms)
	})

	t.Run("MultiPlatform", func(t *testing.T) {
		project := makeProject()
		require.NoError(t, applyPlatforms(project, false))
		require.EqualValues(t, []string{"linux/amd64", "linux/arm64", "alice/32"},
			project.Services["test"].Build.Platforms)
	})
}

func TestApplyPlatforms_DockerDefaultPlatform(t *testing.T) {
	makeProject := func() *types.Project {
		return &types.Project{
			Environment: map[string]string{
				"DOCKER_DEFAULT_PLATFORM": "linux/amd64",
			},
			Services: types.Services{
				"test": {
					Name:  "test",
					Image: "foo",
					Build: &types.BuildConfig{
						Context: ".",
						Platforms: []string{
							"linux/amd64",
							"linux/arm64",
						},
					},
				},
			},
		}
	}

	t.Run("SinglePlatform", func(t *testing.T) {
		project := makeProject()
		require.NoError(t, applyPlatforms(project, true))
		require.EqualValues(t, []string{"linux/amd64"}, project.Services["test"].Build.Platforms)
	})

	t.Run("MultiPlatform", func(t *testing.T) {
		project := makeProject()
		require.NoError(t, applyPlatforms(project, false))
		require.EqualValues(t, []string{"linux/amd64", "linux/arm64"},
			project.Services["test"].Build.Platforms)
	})
}

func TestApplyPlatforms_UnsupportedPlatform(t *testing.T) {
	makeProject := func() *types.Project {
		return &types.Project{
			Environment: map[string]string{
				"DOCKER_DEFAULT_PLATFORM": "commodore/64",
			},
			Services: types.Services{
				"test": {
					Name:  "test",
					Image: "foo",
					Build: &types.BuildConfig{
						Context: ".",
						Platforms: []string{
							"linux/amd64",
							"linux/arm64",
						},
					},
				},
			},
		}
	}

	t.Run("SinglePlatform", func(t *testing.T) {
		project := makeProject()
		require.EqualError(t, applyPlatforms(project, true),
			`service "test" build.platforms does not support value set by DOCKER_DEFAULT_PLATFORM: commodore/64`)
	})

	t.Run("MultiPlatform", func(t *testing.T) {
		project := makeProject()
		require.EqualError(t, applyPlatforms(project, false),
			`service "test" build.platforms does not support value set by DOCKER_DEFAULT_PLATFORM: commodore/64`)
	})
}

func TestIsRemoteConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cli := mocks.NewMockCli(ctrl)

	tests := []struct {
		name        string
		configPaths []string
		want        bool
	}{
		{
			name:        "empty config paths",
			configPaths: []string{},
			want:        false,
		},
		{
			name:        "local file",
			configPaths: []string{"docker-compose.yaml"},
			want:        false,
		},
		{
			name:        "OCI reference",
			configPaths: []string{"oci://registry.example.com/stack:latest"},
			want:        true,
		},
		{
			name:        "GIT reference",
			configPaths: []string{"git://github.com/user/repo.git"},
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := buildOptions{
				ProjectOptions: &ProjectOptions{
					ConfigPaths: tt.configPaths,
				},
			}
			got := isRemoteConfig(cli, opts)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDisplayLocationRemoteStack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cli := mocks.NewMockCli(ctrl)

	buf := new(bytes.Buffer)
	cli.EXPECT().Out().Return(streams.NewOut(buf)).AnyTimes()

	project := &types.Project{
		Name:       "test-project",
		WorkingDir: "/tmp/test",
	}

	options := buildOptions{
		ProjectOptions: &ProjectOptions{
			ConfigPaths: []string{"oci://registry.example.com/stack:latest"},
		},
	}

	displayLocationRemoteStack(cli, project, options)

	output := buf.String()
	require.Equal(t, output, fmt.Sprintf("Your compose stack %q is stored in %q\n", "oci://registry.example.com/stack:latest", "/tmp/test"))
}

func TestDisplayInterpolationVariables(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "compose-test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a temporary compose file
	composeContent := `
services:
  app:
    image: nginx
    environment:
      - TEST_VAR=${TEST_VAR:?required}  # required with default
      - API_KEY=${API_KEY:?}            # required without default
      - DEBUG=${DEBUG:-true}            # optional with default
      - UNSET_VAR                       # optional without default
`
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	err = os.WriteFile(composePath, []byte(composeContent), 0o644)
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cli := mocks.NewMockCli(ctrl)
	cli.EXPECT().Out().Return(streams.NewOut(buf)).AnyTimes()

	// Create ProjectOptions with the temporary compose file
	projectOptions := &ProjectOptions{
		ConfigPaths: []string{composePath},
	}

	// Set up the context with necessary environment variables
	ctx := context.Background()
	_ = os.Setenv("TEST_VAR", "test-value")
	_ = os.Setenv("API_KEY", "123456")
	defer func() {
		_ = os.Unsetenv("TEST_VAR")
		_ = os.Unsetenv("API_KEY")
	}()

	// Extract variables from the model
	info, noVariables, err := extractInterpolationVariablesFromModel(ctx, cli, projectOptions, []string{})
	require.NoError(t, err)
	require.False(t, noVariables)

	// Display the variables
	displayInterpolationVariables(cli.Out(), info)

	// Expected output format with proper spacing
	expected := "\nFound the following variables in configuration:\n" +
		"VARIABLE   VALUE       SOURCE        REQUIRED   DEFAULT\n" +
		"API_KEY    123456      environment   yes         \n" +
		"DEBUG      true       compose file  no         true\n" +
		"TEST_VAR   test-value  environment   yes         \n"

	// Normalize spaces and newlines for comparison
	normalizeSpaces := func(s string) string {
		// Replace multiple spaces with a single space
		s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
		return s
	}

	actualOutput := buf.String()

	// Compare normalized strings
	require.Equal(t,
		normalizeSpaces(expected),
		normalizeSpaces(actualOutput),
		"\nExpected:\n%s\nGot:\n%s", expected, actualOutput)
}

func TestConfirmRemoteIncludes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cli := mocks.NewMockCli(ctrl)

	tests := []struct {
		name       string
		opts       buildOptions
		assumeYes  bool
		userInput  string
		wantErr    bool
		errMessage string
		wantPrompt bool
		wantOutput string
	}{
		{
			name: "no remote includes",
			opts: buildOptions{
				ProjectOptions: &ProjectOptions{
					ConfigPaths: []string{
						"docker-compose.yaml",
						"./local/path/compose.yaml",
					},
				},
			},
			assumeYes:  false,
			wantErr:    false,
			wantPrompt: false,
		},
		{
			name: "assume yes with remote includes",
			opts: buildOptions{
				ProjectOptions: &ProjectOptions{
					ConfigPaths: []string{
						"oci://registry.example.com/stack:latest",
						"git://github.com/user/repo.git",
					},
				},
			},
			assumeYes:  true,
			wantErr:    false,
			wantPrompt: false,
		},
		{
			name: "user confirms remote includes",
			opts: buildOptions{
				ProjectOptions: &ProjectOptions{
					ConfigPaths: []string{
						"oci://registry.example.com/stack:latest",
						"git://github.com/user/repo.git",
					},
				},
			},
			assumeYes:  false,
			userInput:  "y\n",
			wantErr:    false,
			wantPrompt: true,
			wantOutput: "\nWarning: This Compose project includes files from remote sources:\n" +
				"  - oci://registry.example.com/stack:latest\n" +
				"  - git://github.com/user/repo.git\n" +
				"\nRemote includes could potentially be malicious. Make sure you trust the source.\n" +
				"Do you want to continue? [y/N]: ",
		},
		{
			name: "user rejects remote includes",
			opts: buildOptions{
				ProjectOptions: &ProjectOptions{
					ConfigPaths: []string{
						"oci://registry.example.com/stack:latest",
					},
				},
			},
			assumeYes:  false,
			userInput:  "n\n",
			wantErr:    true,
			errMessage: "operation cancelled by user",
			wantPrompt: true,
			wantOutput: "\nWarning: This Compose project includes files from remote sources:\n" +
				"  - oci://registry.example.com/stack:latest\n" +
				"\nRemote includes could potentially be malicious. Make sure you trust the source.\n" +
				"Do you want to continue? [y/N]: ",
		},
	}

	buf := new(bytes.Buffer)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli.EXPECT().Out().Return(streams.NewOut(buf)).AnyTimes()

			if tt.wantPrompt {
				inbuf := io.NopCloser(bytes.NewBufferString(tt.userInput))
				cli.EXPECT().In().Return(streams.NewIn(inbuf)).AnyTimes()
			}

			err := confirmRemoteIncludes(cli, tt.opts, tt.assumeYes)

			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, tt.errMessage, err.Error())
			} else {
				require.NoError(t, err)
			}

			if tt.wantOutput != "" {
				require.Equal(t, tt.wantOutput, buf.String())
			}
			buf.Reset()
		})
	}
}
