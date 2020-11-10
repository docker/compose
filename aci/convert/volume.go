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
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose-cli/aci/login"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/errdefs"
)

const (
	azureFileDriverName            = "azure_file"
	volumeDriveroptsShareNameKey   = "share_name"
	volumeDriveroptsAccountNameKey = "storage_account_name"
	volumeReadOnly                 = "read_only"
)

func (p projectAciHelper) getAciFileVolumes(ctx context.Context, helper login.StorageLogin) ([]containerinstance.Volume, error) {
	var azureFileVolumesSlice []containerinstance.Volume
	for name, v := range p.Volumes {
		if v.Driver == azureFileDriverName {
			shareName, ok := v.DriverOpts[volumeDriveroptsShareNameKey]
			if !ok {
				return nil, fmt.Errorf("cannot retrieve fileshare name for Azurefile")
			}
			accountName, ok := v.DriverOpts[volumeDriveroptsAccountNameKey]
			if !ok {
				return nil, fmt.Errorf("cannot retrieve account name for Azurefile")
			}
			readOnly, ok := v.DriverOpts[volumeReadOnly]
			if !ok {
				readOnly = "false"
			}
			ro, err := strconv.ParseBool(readOnly)
			if err != nil {
				return nil, fmt.Errorf("invalid mode %q for volume", readOnly)
			}
			accountKey, err := helper.GetAzureStorageAccountKey(ctx, accountName)
			if err != nil {
				return nil, err
			}
			aciVolume := containerinstance.Volume{
				Name: to.StringPtr(name),
				AzureFile: &containerinstance.AzureFileVolume{
					ShareName:          to.StringPtr(shareName),
					StorageAccountName: to.StringPtr(accountName),
					StorageAccountKey:  to.StringPtr(accountKey),
					ReadOnly:           &ro,
				},
			}
			azureFileVolumesSlice = append(azureFileVolumesSlice, aciVolume)
		}
	}
	return azureFileVolumesSlice, nil
}

func (s serviceConfigAciHelper) getAciFileVolumeMounts() ([]containerinstance.VolumeMount, error) {
	var aciServiceVolumes []containerinstance.VolumeMount
	for _, sv := range s.Volumes {
		if sv.Type == string(types.VolumeTypeBind) {
			return []containerinstance.VolumeMount{}, fmt.Errorf("host path (%q) not allowed as volume source, you need to reference an Azure File Share defined in the 'volumes' section", sv.Source)
		}
		aciServiceVolumes = append(aciServiceVolumes, containerinstance.VolumeMount{
			Name:      to.StringPtr(sv.Source),
			MountPath: to.StringPtr(sv.Target),
		})
	}
	return aciServiceVolumes, nil
}

// GetRunVolumes return volume configurations for a project and a single service
// this is meant to be used as a compose project of a single service
func GetRunVolumes(volumes []string) (map[string]types.VolumeConfig, []types.ServiceVolumeConfig, error) {
	var serviceConfigVolumes []types.ServiceVolumeConfig
	projectVolumes := make(map[string]types.VolumeConfig, len(volumes))
	for i, v := range volumes {
		var vi volumeInput
		err := vi.parse(fmt.Sprintf("volume-%d", i), v)
		if err != nil {
			return nil, nil, err
		}
		readOnly := strconv.FormatBool(vi.readonly)
		projectVolumes[vi.name] = types.VolumeConfig{
			Name:   vi.name,
			Driver: azureFileDriverName,
			DriverOpts: map[string]string{
				volumeDriveroptsAccountNameKey: vi.storageAccount,
				volumeDriveroptsShareNameKey:   vi.fileshare,
				volumeReadOnly:                 readOnly,
			},
		}
		sv := types.ServiceVolumeConfig{
			Type:     azureFileDriverName,
			Source:   vi.name,
			Target:   vi.target,
			ReadOnly: vi.readonly,
		}
		serviceConfigVolumes = append(serviceConfigVolumes, sv)
	}

	return projectVolumes, serviceConfigVolumes, nil
}

type volumeInput struct {
	name           string
	storageAccount string
	fileshare      string
	target         string
	readonly       bool
}

// parse takes a candidate string and creates a volumeInput
// Candidates take the form of <source>[:<target>][:<permissions>]
// Source is of the form `<storage account>/<fileshare>`
// If only the source is specified then the target is set to `/run/volumes/<fileshare>`
// Target is an absolute path in the container of the form `/path/to/mount`
// Permissions can only be set if the target is set
// If set, permissions must be `rw` or `ro`
func (v *volumeInput) parse(name string, candidate string) error {
	v.name = name

	tokens := strings.Split(candidate, ":")

	sourceTokens := strings.Split(tokens[0], "/")
	if len(sourceTokens) != 2 || sourceTokens[0] == "" {
		return errors.Wrapf(errdefs.ErrParsingFailed, "volume specification %q does not include a storage account before '/'", candidate)
	}
	if sourceTokens[1] == "" {
		return errors.Wrapf(errdefs.ErrParsingFailed, "volume specification %q does not include a storage file fileshare after '/'", candidate)
	}
	v.storageAccount = sourceTokens[0]
	v.fileshare = sourceTokens[1]

	switch len(tokens) {
	case 1: // source only
		v.target = "/run/volumes/" + v.fileshare
	case 2: // source and target
		v.target = tokens[1]
	case 3: // source, target, and permissions
		v.target = tokens[1]
		permissions := strings.ToLower(tokens[2])
		if permissions != "ro" && permissions != "rw" {
			return errors.Wrapf(errdefs.ErrParsingFailed, "volume specification %q has an invalid mode %q", candidate, permissions)
		}
		v.readonly = permissions == "ro"
	default:
		return errors.Wrapf(errdefs.ErrParsingFailed, "volume specification %q has invalid format", candidate)
	}

	return nil
}
