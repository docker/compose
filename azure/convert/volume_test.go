/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package convert

import (
	"testing"

	"github.com/compose-spec/compose-go/types"
	"gotest.tools/assert"

	"github.com/docker/api/errdefs"
)

const (
	storageAccountNameKey = "storage_account_name"
	storageAccountKeyKey  = "storage_account_key"
	shareNameKey          = "share_name"
)

func TestGetRunVolumes(t *testing.T) {
	volumeStrings := []string{
		"myuser1:mykey1@myshare1/my/path/to/target1",
		"myuser2:mykey2@myshare2/my/path/to/target2",
		"myuser3:mykey3@mydefaultsharename", // Use default placement at '/run/volumes/<share_name>'
	}
	var goldenVolumeConfigs = map[string]types.VolumeConfig{
		"volume-0": {
			Name:   "volume-0",
			Driver: "azure_file",
			DriverOpts: map[string]string{
				storageAccountNameKey: "myuser1",
				storageAccountKeyKey:  "mykey1",
				shareNameKey:          "myshare1",
			},
		},
		"volume-1": {
			Name:   "volume-1",
			Driver: "azure_file",
			DriverOpts: map[string]string{
				storageAccountNameKey: "myuser2",
				storageAccountKeyKey:  "mykey2",
				shareNameKey:          "myshare2",
			},
		},
		"volume-2": {
			Name:   "volume-2",
			Driver: "azure_file",
			DriverOpts: map[string]string{
				storageAccountNameKey: "myuser3",
				storageAccountKeyKey:  "mykey3",
				shareNameKey:          "mydefaultsharename",
			},
		},
	}
	goldenServiceVolumeConfigs := []types.ServiceVolumeConfig{
		{
			Type:   "azure_file",
			Source: "volume-0",
			Target: "/my/path/to/target1",
		},
		{
			Type:   "azure_file",
			Source: "volume-1",
			Target: "/my/path/to/target2",
		},
		{
			Type:   "azure_file",
			Source: "volume-2",
			Target: "/run/volumes/mydefaultsharename",
		},
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
	_, _, err := GetRunVolumes([]string{"myuser:mykey@"})
	assert.Equal(t, true, errdefs.IsErrParsingFailed(err))
	assert.ErrorContains(t, err, "does not include a storage file share")
}

func TestGetRunVolumesMissingUser(t *testing.T) {
	_, _, err := GetRunVolumes([]string{":mykey@myshare"})
	assert.Equal(t, true, errdefs.IsErrParsingFailed(err))
	assert.ErrorContains(t, err, "does not include a storage username")
}

func TestGetRunVolumesMissingKey(t *testing.T) {
	_, _, err := GetRunVolumes([]string{"userwithnokey:@myshare"})
	assert.Equal(t, true, errdefs.IsErrParsingFailed(err))
	assert.ErrorContains(t, err, "does not include a storage key")

	_, _, err = GetRunVolumes([]string{"userwithnokeytoo@myshare"})
	assert.Equal(t, true, errdefs.IsErrParsingFailed(err))
	assert.ErrorContains(t, err, "does not include a storage key")
}

func TestGetRunVolumesNoShare(t *testing.T) {
	_, _, err := GetRunVolumes([]string{"noshare"})
	assert.Equal(t, true, errdefs.IsErrParsingFailed(err))
	assert.ErrorContains(t, err, "no share specified")
}
