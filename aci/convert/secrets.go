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
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/pkg/errors"
)

const (
	defaultSecretsPath         = "/run/secrets"
	serviceSecretAbsPathPrefix = "aci-service-secret-path-"
)

func getServiceSecretKey(serviceName, targetDir string) string {
	return fmt.Sprintf("%s-%s--%s",
		serviceSecretAbsPathPrefix, serviceName, strings.ReplaceAll(targetDir, "/", "-"))
}

func (p projectAciHelper) getAciSecretVolumes() ([]containerinstance.Volume, error) {
	var secretVolumes []containerinstance.Volume
	for _, svc := range p.Services {
		squashedTargetVolumes := make(map[string]containerinstance.Volume)
		for _, scr := range svc.Secrets {
			data, err := ioutil.ReadFile(p.Secrets[scr.Source].File)
			if err != nil {
				return secretVolumes, err
			}
			if len(data) == 0 {
				continue
			}
			dataStr := base64.StdEncoding.EncodeToString(data)
			if scr.Target == "" {
				scr.Target = scr.Source
			}

			if !path.IsAbs(scr.Target) && strings.ContainsAny(scr.Target, "\\/") {
				return []containerinstance.Volume{},
					errors.Errorf("in service %q, secret with source %q cannot have a relative path as target. "+
						"Only absolute paths are allowed. Found %q",
						svc.Name, scr.Source, scr.Target)
			}

			if !path.IsAbs(scr.Target) {
				scr.Target = path.Join(defaultSecretsPath, scr.Target)
			}

			targetDir := path.Dir(scr.Target)
			targetDirKey := getServiceSecretKey(svc.Name, targetDir)
			if _, ok := squashedTargetVolumes[targetDir]; !ok {
				squashedTargetVolumes[targetDir] = containerinstance.Volume{
					Name:   to.StringPtr(targetDirKey),
					Secret: make(map[string]*string),
				}
			}

			squashedTargetVolumes[targetDir].Secret[path.Base(scr.Target)] = &dataStr
		}
		for _, v := range squashedTargetVolumes {
			secretVolumes = append(secretVolumes, v)
		}
	}

	return secretVolumes, nil
}

func (s serviceConfigAciHelper) getAciSecretsVolumeMounts() ([]containerinstance.VolumeMount, error) {
	vms := []containerinstance.VolumeMount{}
	presenceSet := make(map[string]bool)
	for _, scr := range s.Secrets {
		if scr.Target == "" {
			scr.Target = scr.Source
		}
		if !path.IsAbs(scr.Target) {
			scr.Target = path.Join(defaultSecretsPath, scr.Target)
		}

		presenceKey := path.Dir(scr.Target)
		if !presenceSet[presenceKey] {
			vms = append(vms, containerinstance.VolumeMount{
				Name:      to.StringPtr(getServiceSecretKey(s.Name, path.Dir(scr.Target))),
				MountPath: to.StringPtr(path.Dir(scr.Target)),
				ReadOnly:  to.BoolPtr(true),
			})
			presenceSet[presenceKey] = true
		}
	}
	err := validateMountPathCollisions(vms)
	if err != nil {
		return []containerinstance.VolumeMount{}, err
	}
	return vms, nil
}

func validateMountPathCollisions(vms []containerinstance.VolumeMount) error {
	for i, vm1 := range vms {
		for j, vm2 := range vms {
			if i == j {
				continue
			}
			var (
				biggerVMPath  = strings.Split(*vm1.MountPath, "/")
				smallerVMPath = strings.Split(*vm2.MountPath, "/")
			)
			if len(smallerVMPath) > len(biggerVMPath) {
				tmp := biggerVMPath
				biggerVMPath = smallerVMPath
				smallerVMPath = tmp
			}
			isPrefixed := true
			for i := 0; i < len(smallerVMPath); i++ {
				if smallerVMPath[i] != biggerVMPath[i] {
					isPrefixed = false
					break
				}
			}
			if isPrefixed {
				return errors.Errorf("mount paths %q and %q collide. A volume mount cannot include another one.", *vm1.MountPath, *vm2.MountPath)
			}
		}
	}
	return nil
}
