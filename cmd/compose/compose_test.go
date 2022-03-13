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
	"os"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"gotest.tools/v3/assert"
)

func TestFilterServices(t *testing.T) {
	p := &types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "foo",
				Links: []string{"bar"},
			},
			{
				Name:        "bar",
				NetworkMode: types.NetworkModeServicePrefix + "zot",
			},
			{
				Name: "zot",
			},
			{
				Name: "qix",
			},
		},
	}
	err := p.ForServices([]string{"bar"})
	assert.NilError(t, err)

	assert.Equal(t, len(p.Services), 2)
	_, err = p.GetService("bar")
	assert.NilError(t, err)
	_, err = p.GetService("zot")
	assert.NilError(t, err)
}

func TestPrintParametersWithEnvFile(t *testing.T) {
	dirName, err := os.MkdirTemp("", "docker_compose_dir")
	if err != nil {
		assert.NilError(t, err)
	}

	defer func() {
		err := os.RemoveAll(dirName)
		assert.NilError(t, err)
	}()

	dockerCompFile, err := os.CreateTemp(dirName, "docker-compose-*.yml")

	if err != nil {
		assert.NilError(t, err)
	}
	defer func() {
		err := dockerCompFile.Close()
		assert.NilError(t, err)
	}()

	defer func() {
		err := os.Remove(dockerCompFile.Name())
		assert.NilError(t, err)
	}()

	_, err = dockerCompFile.WriteString(`
services:
  redis:
    image: redis
  pg:
    networks:
      - backend
    image: postgres
    command: "${LOGGIN_LEVEL:-log_info}"
networks:
  frontend:
    driver: custom-driver-1
  backend:
    driver: custom-driver-2
    driver_opts:
      foo: "${FOO_TEST_ENV_VAR}"
      bar: "${BAR?error}"
      buzz: "${BUZZ:-buzz}"`)

	assert.NilError(t, err)

	envFile, err := os.CreateTemp(dirName, "env_file-*")
	assert.NilError(t, err)

	defer func() {
		err := envFile.Close()
		assert.NilError(t, err)
	}()
	defer func() {
		err := os.Remove(envFile.Name())
		assert.NilError(t, err)
	}()

	_, err = envFile.WriteString("FOO_TEST_ENV_VAR=FILE_FOO_VALUE\nBAR=BAR_VALUE\n")
	assert.NilError(t, err)

	err = os.Setenv("FOO_TEST_ENV_VAR", "ENV_FOO_VALUE")
	assert.NilError(t, err)

	defer func() {
		err := os.Unsetenv("FOO_TEST_ENV_VAR")
		assert.NilError(t, err)
	}()

	p := projectOptions{
		ConfigPaths: []string{dockerCompFile.Name()},
		EnvFile:     envFile.Name(),
		ProjectDir:  dirName,
	}
	opts := convertOptions{
		projectOptions: &p,
	}
	params, err := getParameters(opts)

	assert.NilError(t, err)

	expectedMap := Parameters{
		"BAR": Parameter{
			Name:     "BAR",
			Default:  "",
			Required: true,
			Actual:   "BAR_VALUE",
			Source:   envFile.Name(),
		},
		"BUZZ": Parameter{
			Name:     "BUZZ",
			Default:  "buzz",
			Required: false,
			Actual:   "buzz",
			Source:   dockerCompFile.Name(),
		},
		"FOO_TEST_ENV_VAR": Parameter{
			Name:     "FOO_TEST_ENV_VAR",
			Default:  "",
			Required: false,
			Actual:   "ENV_FOO_VALUE",
			Source:   "os.Env",
		},
		"LOGGIN_LEVEL": Parameter{
			Name:     "LOGGIN_LEVEL",
			Default:  "log_info",
			Required: false,
			Actual:   "log_info",
			Source:   dockerCompFile.Name(),
		},
	}
	assert.Equal(t, len(params), len(expectedMap))
	for key, val := range expectedMap {
		assert.Equal(t, params[key], val)
	}
}

func TestPrintParametersWithDotFile(t *testing.T) {
	dirName, err := os.MkdirTemp("", "docker_compose_dir")
	assert.NilError(t, err)

	defer func() {
		err := os.RemoveAll(dirName)
		assert.NilError(t, err)
	}()

	dockerCompFile, err := os.CreateTemp(dirName, "docker-compose-*.yml")
	assert.NilError(t, err)

	defer func() {
		err := dockerCompFile.Close()
		assert.NilError(t, err)
	}()
	defer func() {
		err := os.Remove(dockerCompFile.Name())
		assert.NilError(t, err)
	}()

	_, err = dockerCompFile.WriteString(`
services:
  redis:
    image: redis
  pg:
    networks:
      - backend
    image: postgres
    command: "${LOGGIN_LEVEL:-log_info}"
networks:
  frontend:
    driver: custom-driver-1
  backend:
    driver: custom-driver-2
    driver_opts:
      foo: "${FOO}"
      bar: "${BAR?error}"
      buzz: "${BUZZ:-buzz}"`)

	assert.NilError(t, err)

	envFile, err := os.Create(dirName + "/" + ".env")
	assert.NilError(t, err)

	defer func() {
		err := envFile.Close()
		assert.NilError(t, err)
	}()
	defer func() {
		err := os.Remove(envFile.Name())
		assert.NilError(t, err)
	}()

	_, err = envFile.WriteString("FOO=FOO_VALUE\nBAR=BAR_VALUE\n")
	assert.NilError(t, err)

	p := projectOptions{
		ConfigPaths: []string{dockerCompFile.Name()},
		ProjectDir:  dirName,
	}
	opts := convertOptions{
		projectOptions: &p,
	}
	params, err := getParameters(opts)

	assert.NilError(t, err)

	expectedMap := Parameters{
		"BAR": Parameter{
			Name:     "BAR",
			Default:  "",
			Required: true,
			Actual:   "BAR_VALUE",
			Source:   ".env",
		},
		"BUZZ": Parameter{
			Name:     "BUZZ",
			Default:  "buzz",
			Required: false,
			Actual:   "buzz",
			Source:   dockerCompFile.Name(),
		},
		"FOO": Parameter{
			Name:     "FOO",
			Default:  "",
			Required: false,
			Actual:   "FOO_VALUE",
			Source:   ".env",
		},
		"LOGGIN_LEVEL": Parameter{
			Name:     "LOGGIN_LEVEL",
			Default:  "log_info",
			Required: false,
			Actual:   "log_info",
			Source:   dockerCompFile.Name(),
		},
	}
	assert.Equal(t, len(params), len(expectedMap))
	for key, val := range expectedMap {
		assert.Equal(t, params[key], val)
	}
}
