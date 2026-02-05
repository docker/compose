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
	"testing"

	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

func TestSetEnvWithDotEnv_WithOCIArtifact(t *testing.T) {
	// Test that setEnvWithDotEnv doesn't fail when using OCI artifacts
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cli := mocks.NewMockCli(ctrl)

	opts := ProjectOptions{
		ConfigPaths: []string{"oci://docker.io/dockersamples/welcome-to-docker"},
		ProjectDir:  "",
		EnvFiles:    []string{},
	}

	err := setEnvWithDotEnv(opts, cli)
	assert.NilError(t, err, "setEnvWithDotEnv should not fail with OCI artifact path")
}

func TestSetEnvWithDotEnv_WithGitRemote(t *testing.T) {
	// Test that setEnvWithDotEnv doesn't fail when using Git remotes
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cli := mocks.NewMockCli(ctrl)

	opts := ProjectOptions{
		ConfigPaths: []string{"https://github.com/docker/compose.git"},
		ProjectDir:  "",
		EnvFiles:    []string{},
	}

	err := setEnvWithDotEnv(opts, cli)
	assert.NilError(t, err, "setEnvWithDotEnv should not fail with Git remote path")
}

func TestSetEnvWithDotEnv_WithLocalPath(t *testing.T) {
	// Test that setEnvWithDotEnv still works with local paths
	// This will fail if the file doesn't exist, but it should not panic
	// or produce invalid paths
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cli := mocks.NewMockCli(ctrl)

	opts := ProjectOptions{
		ConfigPaths: []string{"compose.yaml"},
		ProjectDir:  "",
		EnvFiles:    []string{},
	}

	// This may error if files don't exist, but should not panic
	_ = setEnvWithDotEnv(opts, cli)
}
