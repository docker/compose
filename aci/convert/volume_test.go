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
	"strconv"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"gotest.tools/v3/assert"
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
			volumeDriveroptsAccountNameKey: accountNameKey,
			volumeDriveroptsShareNameKey:   shareNameKey,
			volumeReadOnly:                 strconv.FormatBool(readOnly),
		},
	}
}
