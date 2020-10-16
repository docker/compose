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
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"gotest.tools/v3/assert"
)

func TestConvertSecrets(t *testing.T) {
	serviceName := "testservice"
	secretName := "testsecret"
	absBasePath := "/home/user"
	tmpFile, err := ioutil.TempFile(os.TempDir(), "TestConvertProjectSecrets-")
	assert.NilError(t, err)
	_, err = tmpFile.Write([]byte("test content"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = os.Remove(tmpFile.Name())
	})

	t.Run("mix default and absolute", func(t *testing.T) {
		pSquashedDefaultAndAbs := projectAciHelper{
			Services: []types.ServiceConfig{
				{
					Name: serviceName,
					Secrets: []types.ServiceSecretConfig{
						{
							Source: secretName,
							Target: "some_target1",
						},
						{
							Source: secretName,
						},
						{
							Source: secretName,
							Target: path.Join(defaultSecretsPath, "some_target2"),
						},
						{
							Source: secretName,
							Target: path.Join(absBasePath, "some_target3"),
						},
						{
							Source: secretName,
							Target: path.Join(absBasePath, "some_target4"),
						},
					},
				},
			},
			Secrets: map[string]types.SecretConfig{
				secretName: {
					File: tmpFile.Name(),
				},
			},
		}
		volumes, err := pSquashedDefaultAndAbs.getAciSecretVolumes()
		assert.NilError(t, err)
		assert.Equal(t, len(volumes), 2)

		defaultVolumeName := getServiceSecretKey(serviceName, defaultSecretsPath)
		homeVolumeName := getServiceSecretKey(serviceName, absBasePath)
		// random order since this was created from a map...
		for _, vol := range volumes {
			switch *vol.Name {
			case defaultVolumeName:
				assert.Equal(t, len(vol.Secret), 3)
			case homeVolumeName:
				assert.Equal(t, len(vol.Secret), 2)
			default:
				assert.Assert(t, false, "unexpected volume name: "+*vol.Name)
			}
		}

		s := serviceConfigAciHelper(pSquashedDefaultAndAbs.Services[0])
		vms, err := s.getAciSecretsVolumeMounts()
		assert.NilError(t, err)
		assert.Equal(t, len(vms), 2)

		assert.Equal(t, *vms[0].Name, defaultVolumeName)
		assert.Equal(t, *vms[0].MountPath, defaultSecretsPath)

		assert.Equal(t, *vms[1].Name, homeVolumeName)
		assert.Equal(t, *vms[1].MountPath, absBasePath)
	})

	t.Run("convert invalid target", func(t *testing.T) {
		targetName := "some/invalid/relative/path/target"
		pInvalidRelativePathTarget := projectAciHelper{
			Services: []types.ServiceConfig{
				{
					Name: serviceName,
					Secrets: []types.ServiceSecretConfig{
						{
							Source: secretName,
							Target: targetName,
						},
					},
				},
			},
			Secrets: map[string]types.SecretConfig{
				secretName: {
					File: tmpFile.Name(),
				},
			},
		}
		_, err := pInvalidRelativePathTarget.getAciSecretVolumes()
		assert.Equal(t, err.Error(),
			fmt.Sprintf(`in service %q, secret with source %q cannot have a relative path as target. Only absolute paths are allowed. Found %q`,
				serviceName, secretName, targetName))
	})

	t.Run("convert colliding default targets", func(t *testing.T) {
		targetName1 := path.Join(defaultSecretsPath, "target1")
		targetName2 := path.Join(defaultSecretsPath, "sub/folder/target2")

		service := serviceConfigAciHelper{
			Name: serviceName,
			Secrets: []types.ServiceSecretConfig{
				{
					Source: secretName,
					Target: targetName1,
				},
				{
					Source: secretName,
					Target: targetName2,
				},
			},
		}

		_, err := service.getAciSecretsVolumeMounts()
		assert.Equal(t, err.Error(),
			fmt.Sprintf(`mount paths %q and %q collide. A volume mount cannot include another one.`,
				path.Dir(targetName1), path.Dir(targetName2)))
	})

	t.Run("convert colliding absolute targets", func(t *testing.T) {
		targetName1 := path.Join(absBasePath, "target1")
		targetName2 := path.Join(absBasePath, "sub/folder/target2")

		service := serviceConfigAciHelper{
			Name: serviceName,
			Secrets: []types.ServiceSecretConfig{
				{
					Source: secretName,
					Target: targetName1,
				},
				{
					Source: secretName,
					Target: targetName2,
				},
			},
		}

		_, err := service.getAciSecretsVolumeMounts()
		assert.Equal(t, err.Error(),
			fmt.Sprintf(`mount paths %q and %q collide. A volume mount cannot include another one.`,
				path.Dir(targetName1), path.Dir(targetName2)))
	})
}
