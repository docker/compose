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

package convert

import (
	"context"
	"strconv"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"
	"github.com/stretchr/testify/mock"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func TestGetRunVolumes(t *testing.T) {
	volumeStrings := []string{
		"myuser1/myshare1:/my/path/to/target1",
		"myuser2/myshare2:/my/path/to/target2:ro",
		"myuser3/myshare3:/my/path/to/target3:rw",
		"myuser4/mydefaultsharename", // Use default placement at '/run/volumes/<share_name>'
	}
	var goldenVolumeConfigs = map[string]types.VolumeConfig{
		"volume-0": getAzurefileVolumeConfig("volume-0", "myuser1", "myshare1", false),
		"volume-1": getAzurefileVolumeConfig("volume-1", "myuser2", "myshare2", true),
		"volume-2": getAzurefileVolumeConfig("volume-2", "myuser3", "myshare3", false),
		"volume-3": getAzurefileVolumeConfig("volume-3", "myuser4", "mydefaultsharename", false),
	}
	goldenServiceVolumeConfigs := []types.ServiceVolumeConfig{
		getServiceVolumeConfig("volume-0", "/my/path/to/target1", false),
		getServiceVolumeConfig("volume-1", "/my/path/to/target2", true),
		getServiceVolumeConfig("volume-2", "/my/path/to/target3", false),
		getServiceVolumeConfig("volume-3", "/run/volumes/mydefaultsharename", false),
	}

	volumeConfigs, serviceVolumeConfigs, err := GetRunVolumes(volumeStrings)
	assert.NilError(t, err)
	for k, v := range volumeConfigs {
		assert.DeepEqual(t, goldenVolumeConfigs[k], v)
	}
	for i, v := range serviceVolumeConfigs {
		assert.DeepEqual(t, goldenServiceVolumeConfigs[i], v)
	}
}

func TestGetRunVolumesMissingFileShare(t *testing.T) {
	_, _, err := GetRunVolumes([]string{"myaccount/"})
	assert.ErrorContains(t, err, "does not include a storage file fileshare after '/'")
}

func TestGetRunVolumesMissingUser(t *testing.T) {
	_, _, err := GetRunVolumes([]string{"/myshare"})
	assert.ErrorContains(t, err, "does not include a storage account before '/'")
}

func TestGetRunVolumesNoShare(t *testing.T) {
	_, _, err := GetRunVolumes([]string{"noshare"})
	assert.ErrorContains(t, err, "does not include a storage account before '/'")
}

func TestGetRunVolumesInvalidOption(t *testing.T) {
	_, _, err := GetRunVolumes([]string{"myuser4/myshare4:/my/path/to/target4:invalid"})
	assert.ErrorContains(t, err, `volume specification "myuser4/myshare4:/my/path/to/target4:invalid" has an invalid mode "invalid"`)
}

func TestComposeVolumes(t *testing.T) {
	ctx := context.TODO()
	accountName := "myAccount"
	mockStorageHelper.On("GetAzureStorageAccountKey", ctx, accountName).Return("123456", nil)
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
			},
		},
		Volumes: types.Volumes{
			"vol1": types.VolumeConfig{
				Driver: "azure_file",
				DriverOpts: map[string]string{
					"share_name":           "myFileshare",
					"storage_account_name": accountName,
				},
			},
		},
	}

	group, err := ToContainerGroup(ctx, convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)

	assert.Assert(t, is.Len(*group.Containers, 1))
	assert.Equal(t, *(*group.Containers)[0].Name, "service1")
	expectedGroupVolume := containerinstance.Volume{
		Name: to.StringPtr("vol1"),
		AzureFile: &containerinstance.AzureFileVolume{
			ShareName:          to.StringPtr("myFileshare"),
			StorageAccountName: &accountName,
			StorageAccountKey:  to.StringPtr("123456"),
			ReadOnly:           to.BoolPtr(false),
		},
	}
	assert.Equal(t, len(*group.Volumes), 1)
	assert.DeepEqual(t, (*group.Volumes)[0], expectedGroupVolume)
}

func TestPathVolumeErrorMessage(t *testing.T) {
	ctx := context.TODO()
	accountName := "myAccount"
	mockStorageHelper.On("GetAzureStorageAccountKey", ctx, accountName).Return("123456", nil)
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Volumes: []types.ServiceVolumeConfig{
					{
						Source: "/path",
						Target: "/target",
						Type:   string(types.VolumeTypeBind),
					},
				},
			},
		},
	}

	_, err := ToContainerGroup(ctx, convertCtx, project, mockStorageHelper)
	assert.ErrorContains(t, err, `host path ("/path") not allowed as volume source, you need to reference an Azure File Share defined in the 'volumes' section`)
}

func TestComposeVolumesRO(t *testing.T) {
	ctx := context.TODO()
	accountName := "myAccount"
	mockStorageHelper.On("GetAzureStorageAccountKey", ctx, accountName).Return("123456", nil)
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
			},
		},
		Volumes: types.Volumes{
			"vol1": types.VolumeConfig{
				Driver: "azure_file",
				DriverOpts: map[string]string{
					"share_name":           "myFileshare",
					"storage_account_name": accountName,
					"read_only":            "true",
				},
			},
		},
	}

	group, err := ToContainerGroup(ctx, convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)

	assert.Assert(t, is.Len(*group.Containers, 1))
	assert.Equal(t, *(*group.Containers)[0].Name, "service1")
	expectedGroupVolume := containerinstance.Volume{
		Name: to.StringPtr("vol1"),
		AzureFile: &containerinstance.AzureFileVolume{
			ShareName:          to.StringPtr("myFileshare"),
			StorageAccountName: &accountName,
			StorageAccountKey:  to.StringPtr("123456"),
			ReadOnly:           to.BoolPtr(true),
		},
	}
	assert.Equal(t, len(*group.Volumes), 1)
	assert.DeepEqual(t, (*group.Volumes)[0], expectedGroupVolume)
}

type mockStorageLogin struct {
	mock.Mock
}

func (s *mockStorageLogin) GetAzureStorageAccountKey(ctx context.Context, accountName string) (string, error) {
	args := s.Called(ctx, accountName)
	return args.String(0), args.Error(1)
}

func getServiceVolumeConfig(source string, target string, readOnly bool) types.ServiceVolumeConfig {
	return types.ServiceVolumeConfig{
		Type:     "azure_file",
		Source:   source,
		Target:   target,
		ReadOnly: readOnly,
	}
}

func getAzurefileVolumeConfig(name string, accountNameKey string, shareNameKey string, readOnly bool) types.VolumeConfig {
	return types.VolumeConfig{
		Name:   name,
		Driver: "azure_file",
		DriverOpts: map[string]string{
			VolumeDriveroptsAccountNameKey: accountNameKey,
			VolumeDriveroptsShareNameKey:   shareNameKey,
			volumeReadOnly:                 strconv.FormatBool(readOnly),
		},
	}
}
