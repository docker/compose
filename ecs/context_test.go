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

package ecs

import (
	"context"
	"os"
	"testing"

	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/utils/prompt"

	"github.com/golang/mock/gomock"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/fs"
	"gotest.tools/v3/golden"
)

func TestCreateContextDataFromEnv(t *testing.T) {
	c := contextCreateAWSHelper{
		user: nil,
	}
	data, desc, err := c.createContextData(context.TODO(), ContextParams{
		Name:         "test",
		CredsFromEnv: true,
	})
	assert.NilError(t, err)
	assert.Equal(t, data.(store.EcsContext).CredentialsFromEnv, true)
	assert.Equal(t, desc, "credentials read from environment")
}

func TestCreateContextDataByKeys(t *testing.T) {
	dir := fs.NewDir(t, "aws")
	os.Setenv("AWS_CONFIG_FILE", dir.Join("config"))                  // nolint:errcheck
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", dir.Join("credentials")) // nolint:errcheck

	defer os.Unsetenv("AWS_CONFIG_FILE")             // nolint:errcheck
	defer os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE") // nolint:errcheck

	c := contextCreateAWSHelper{
		user: nil,
	}

	data, _, err := c.createContextData(context.TODO(), ContextParams{
		Name:      "test",
		AccessKey: "ABCD",
		SecretKey: "X&123",
		Region:    "eu-west-3",
	})
	assert.NilError(t, err)
	assert.Equal(t, data.(store.EcsContext).Profile, "default")

	s := golden.Get(t, dir.Join("config"))
	golden.Assert(t, string(s), "context/by-keys/config.golden")

	s = golden.Get(t, dir.Join("credentials"))
	golden.Assert(t, string(s), "context/by-keys/credentials.golden")
}

func TestCreateContextDataFromProfile(t *testing.T) {
	os.Setenv("AWS_CONFIG_FILE", "testdata/context/by-profile/config.golden")                  // nolint:errcheck
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", "testdata/context/by-profile/credentials.golden") // nolint:errcheck

	defer os.Unsetenv("AWS_CONFIG_FILE")             // nolint:errcheck
	defer os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE") // nolint:errcheck

	c := contextCreateAWSHelper{
		user: nil,
	}

	data, _, err := c.createContextData(context.TODO(), ContextParams{
		Name:    "test",
		Profile: "foo",
	})
	assert.NilError(t, err)
	assert.Equal(t, data.(store.EcsContext).Profile, "foo")
}

func TestCreateContextDataFromEnvInteractive(t *testing.T) {
	dir := fs.NewDir(t, "aws")
	os.Setenv("AWS_CONFIG_FILE", dir.Join("config"))                  // nolint:errcheck
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", dir.Join("credentials")) // nolint:errcheck

	defer os.Unsetenv("AWS_CONFIG_FILE")             // nolint:errcheck
	defer os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE") // nolint:errcheck

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ui := prompt.NewMockUI(ctrl)
	c := contextCreateAWSHelper{
		user: ui,
	}

	ui.EXPECT().Select("Create a Docker context using:", gomock.Any()).Return(1, nil)
	data, _, err := c.createContextData(context.TODO(), ContextParams{})
	assert.NilError(t, err)
	assert.Equal(t, data.(store.EcsContext).CredentialsFromEnv, true)
}

func TestCreateContextDataByKeysInteractive(t *testing.T) {
	dir := fs.NewDir(t, "aws")
	os.Setenv("AWS_CONFIG_FILE", dir.Join("config"))                  // nolint:errcheck
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", dir.Join("credentials")) // nolint:errcheck

	defer os.Unsetenv("AWS_CONFIG_FILE")             // nolint:errcheck
	defer os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE") // nolint:errcheck

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ui := prompt.NewMockUI(ctrl)
	c := contextCreateAWSHelper{
		user: ui,
		availableRegions: func(opts *ContextParams) ([]string, error) {
			return []string{"us-east-1", "eu-west-3"}, nil
		},
	}

	ui.EXPECT().Select("Create a Docker context using:", gomock.Any()).Return(0, nil)
	ui.EXPECT().Input("AWS Access Key ID", gomock.Any()).Return("ABCD", nil)
	ui.EXPECT().Password("Enter AWS Secret Access Key").Return("X&123", nil)
	ui.EXPECT().Select("Region", []string{"us-east-1", "eu-west-3"}).Return(1, nil)

	data, _, err := c.createContextData(context.TODO(), ContextParams{})
	assert.NilError(t, err)
	assert.Equal(t, data.(store.EcsContext).Profile, "default")

	assert.NilError(t, err)
	assert.Equal(t, data.(store.EcsContext).Profile, "default")

	s := golden.Get(t, dir.Join("config"))
	golden.Assert(t, string(s), "context/by-keys/config.golden")

	s = golden.Get(t, dir.Join("credentials"))
	golden.Assert(t, string(s), "context/by-keys/credentials.golden")
}

func TestCreateContextDataByProfileInteractive(t *testing.T) {
	os.Setenv("AWS_CONFIG_FILE", "testdata/context/by-profile/config.golden")                  // nolint:errcheck
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", "testdata/context/by-profile/credentials.golden") // nolint:errcheck

	defer os.Unsetenv("AWS_CONFIG_FILE")             // nolint:errcheck
	defer os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE") // nolint:errcheck

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ui := prompt.NewMockUI(ctrl)
	c := contextCreateAWSHelper{
		user: ui,
	}
	ui.EXPECT().Select("Create a Docker context using:", gomock.Any()).Return(0, nil)
	ui.EXPECT().Select("Select AWS Profile", []string{"default", "foo"}).Return(1, nil)

	data, _, err := c.createContextData(context.TODO(), ContextParams{})
	assert.NilError(t, err)
	assert.Equal(t, data.(store.EcsContext).Profile, "foo")
}
