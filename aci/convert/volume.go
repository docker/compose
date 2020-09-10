/*
   Copyright 2020 Docker, Inc.

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
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/compose-spec/compose-go/types"

	"github.com/docker/compose-cli/errdefs"
)

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
		projectVolumes[vi.name] = types.VolumeConfig{
			Name:   vi.name,
			Driver: azureFileDriverName,
			DriverOpts: map[string]string{
				volumeDriveroptsAccountNameKey: vi.storageAccount,
				volumeDriveroptsShareNameKey:   vi.fileshare,
			},
		}
		sv := types.ServiceVolumeConfig{
			Type:   azureFileDriverName,
			Source: vi.name,
			Target: vi.target,
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
}

func (v *volumeInput) parse(name string, s string) error {
	v.name = name
	tokens := strings.Split(s, ":")
	source := tokens[0]
	sourceTokens := strings.Split(source, "/")
	if len(sourceTokens) < 2 || sourceTokens[0] == "" {
		return errors.Wrapf(errdefs.ErrParsingFailed, "volume specification %q does not include a storage account before '/'", v)
	}
	v.storageAccount = sourceTokens[0]
	if sourceTokens[1] == "" {
		return errors.Wrapf(errdefs.ErrParsingFailed, "volume specification %q does not include a storage file fileshare after '/'", v)
	}
	v.fileshare = sourceTokens[1]

	if len(tokens) > 1 {
		v.target = tokens[1]
	} else {
		v.target = "/run/volumes/" + v.fileshare
	}
	return nil
}
