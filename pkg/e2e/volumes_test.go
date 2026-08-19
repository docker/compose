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

package e2e

import (
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestLocalComposeVolume(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "compose-e2e-volume"

	t.Run("up with build and no image name, volume", func(t *testing.T) {
		// ensure local test run does not reuse previously build image
		c.RunDockerOrExitError(t, "rmi", "compose-e2e-volume-nginx")
		c.RunDockerOrExitError(t, "volume", "rm", projectName+"-staticVol")
		c.RunDockerOrExitError(t, "volume", "rm", "myvolume")
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/volume-test", "--project-name", projectName, "up",
			"-d")
	})

	t.Run("access bind mount data", func(t *testing.T) {
		output := HTTPGetWithRetry(t, "http://localhost:8090", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, "Hello from Nginx container"))
	})

	t.Run("check container volume specs", func(t *testing.T) {
		res := c.RunDockerCmd(t, "inspect", "compose-e2e-volume-nginx2-1", "--format", "{{ json .Mounts }}")
		output := res.Stdout()
		assert.Assert(t, strings.Contains(output, `"Destination":"/usr/src/app/node_modules","Driver":"local","Mode":"z","RW":true,"Propagation":""`), output)
		assert.Assert(t, strings.Contains(output, `"Destination":"/myconfig","Mode":"","RW":false,"Propagation":"rprivate"`), output)
	})

	t.Run("check config content", func(t *testing.T) {
		output := c.RunDockerCmd(t, "exec", "compose-e2e-volume-nginx2-1", "cat", "/myconfig").Stdout()
		assert.Assert(t, strings.Contains(output, `Hello from Nginx container`), output)
	})

	t.Run("check secrets content", func(t *testing.T) {
		output := c.RunDockerCmd(t, "exec", "compose-e2e-volume-nginx2-1", "cat", "/run/secrets/mysecret").Stdout()
		assert.Assert(t, strings.Contains(output, `Hello from Nginx container`), output)
	})

	t.Run("check container bind-mounts specs", func(t *testing.T) {
		res := c.RunDockerCmd(t, "inspect", "compose-e2e-volume-nginx-1", "--format", "{{ json .Mounts }}")
		output := res.Stdout()
		assert.Assert(t, strings.Contains(output, `"Type":"bind"`))
		assert.Assert(t, strings.Contains(output, `"Destination":"/usr/share/nginx/html"`))
	})

	t.Run("should inherit anonymous volumes", func(t *testing.T) {
		c.RunDockerOrExitError(t, "exec", "compose-e2e-volume-nginx2-1", "touch", "/usr/src/app/node_modules/test")
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/volume-test", "--project-name", projectName, "up", "--force-recreate", "-d")
		c.RunDockerOrExitError(t, "exec", "compose-e2e-volume-nginx2-1", "ls", "/usr/src/app/node_modules/test")
	})

	t.Run("should renew anonymous volumes", func(t *testing.T) {
		c.RunDockerOrExitError(t, "exec", "compose-e2e-volume-nginx2-1", "touch", "/usr/src/app/node_modules/test")
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/volume-test", "--project-name", projectName, "up", "--force-recreate", "--renew-anon-volumes", "-d")
		c.RunDockerOrExitError(t, "exec", "compose-e2e-volume-nginx2-1", "ls", "/usr/src/app/node_modules/test")
	})

	t.Run("cleanup volume project", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "--volumes")
		ls := c.RunDockerCmd(t, "volume", "ls").Stdout()
		assert.Assert(t, !strings.Contains(ls, projectName+"-staticVol"))
		assert.Assert(t, !strings.Contains(ls, "myvolume"))
	})
}

func TestProjectVolumeBind(t *testing.T) {
	if composeStandaloneMode {
		t.Skip()
	}
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-project-volume-bind"

	t.Run("up on project volume with bind specification", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Running on Windows. Skipping...")
		}
		tmpDir := t.TempDir()

		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")

		c.RunDockerOrExitError(t, "volume", "rm", "-f", projectName+"_project-data").Assert(t, icmd.Success)
		cmd := c.NewCmdWithEnv([]string{"TEST_DIR=" + tmpDir},
			"docker", "compose", "--project-directory", "fixtures/project-volume-bind-test", "--project-name", projectName, "up", "-d")
		icmd.RunCmd(cmd).Assert(t, icmd.Success)
		defer c.RunDockerComposeCmd(t, "--project-name", projectName, "down")

		c.RunCmd(t, "sh", "-c", "echo SUCCESS > "+filepath.Join(tmpDir, "resultfile")).Assert(t, icmd.Success)

		ret := c.RunDockerOrExitError(t, "exec", "frontend", "bash", "-c", "cat /data/resultfile").Assert(t, icmd.Success)
		assert.Assert(t, strings.Contains(ret.Stdout(), "SUCCESS"))
	})
}

func TestUpSwitchVolumes(t *testing.T) {
	s := NewScenario(t, "switching a service's external volume must reconnect the container to the new one")
	vol1, vol2 := s.Project()+"-ext-1", s.Project()+"-ext-2"
	s.Defer(
		DockerCmd("volume", "rm", "-f", vol1).MayFail(),
		DockerCmd("volume", "rm", "-f", vol2).MayFail()).
		Step("create the two external volumes",
			DockerCmd("volume", "create", vol1)).
		Step("and the second one",
			DockerCmd("volume", "create", vol2)).
		Step("up mounts the first volume",
			ComposeCmd("up", "-d").WithEnv("EXTERNAL_VOLUME="+vol1)).
		Step("the container's mount points at the first volume",
			DockerCmd("inspect", s.Project()+"-app-1", "-f", "{{ (index .Mounts 0).Name }}"),
			OutputContains(vol1)).
		Step("up with the other volume reconnects the service",
			ComposeCmd("up", "-d").WithEnv("EXTERNAL_VOLUME="+vol2)).
		Step("the container's mount points at the second volume",
			DockerCmd("inspect", s.Project()+"-app-1", "-f", "{{ (index .Mounts 0).Name }}"),
			OutputContains(vol2))
}

func TestUpRecreateVolumes(t *testing.T) {
	s := NewScenario(t, "a volume definition change must recreate the volume on an approved up")
	s.Step("up creates the volume with its declared label",
		ComposeCmd("up", "-d")).
		Step("the volume carries the initial label",
			DockerCmd("volume", "inspect", s.Project()+"_my_vol", "-f", `{{ index .Labels "foo" }}`),
			OutputContains("bar")).
		Step("up -y with a changed label recreates the volume",
			ComposeCmd("up", "-d", "-y").WithEnv("VOL_LABEL=zot")).
		Step("the volume carries the new label",
			DockerCmd("volume", "inspect", s.Project()+"_my_vol", "-f", `{{ index .Labels "foo" }}`),
			OutputContains("zot"))
}

func TestUpRecreateVolumesIgnoreBinds(t *testing.T) {
	NewScenario(t, "a bind mount must not be mistaken for a volume change on an unchanged up").
		Step("up starts the service with its bind mount",
			ComposeCmd("up", "-d")).
		Step("an unchanged up leaves the container alone",
			ComposeCmd("up", "-d"),
			OutputNotContains("Recreated"),
			NotRecreated("app"))
}

func TestImageVolume(t *testing.T) {
	NewScenario(t, "an image volume must mount the source image's content at the requested subpath").
		Requires(EngineVersionAtLeast(28)).
		Step("up mounts the image content",
			ComposeCmd("up", "with_image"),
			OutputContains("index.html"))
}

func TestImageVolumeImageAlreadyLocal(t *testing.T) {
	// Regression test for https://github.com/docker/compose/issues/14005
	// (pulled-image scenario): when the source image of a `type: image` volume
	// is already present locally, compose used to rewrite the mount source to a
	// digest the daemon can't resolve as a mount source under the containerd
	// image store ("No such image"). TestImageVolume only covers this path by
	// accident, when a previous test left the image in the local store.
	NewScenario(t, "an image volume whose source image is already local must mount, and stay idempotent", Serial()).
		Requires(EngineVersionAtLeast(28)).
		Step("have the source image already in the local store",
			DockerCmd("pull", "-q", "nginx:alpine")).
		Step("up mounts the image volume content",
			ComposeCmd("up", "app"),
			OutputContains("index.html")).
		Step("an unchanged up does not recreate the service",
			ComposeCmd("up", "app"),
			OutputNotContains("Recreate"),
			NotRecreated("app"))
}

func TestImageVolumeRecreateOnRebuild(t *testing.T) {
	s := NewScenario(t, "rebuilding an image-volume source must recreate its consumer to expose the new content")
	image := s.Project() + "-source-image"
	s.Requires(EngineVersionAtLeast(28)).
		Env("SOURCE_IMAGE="+image).
		Defer(DockerCmd("image", "rm", "-f", image).MayFail()).
		Step("build the source image with its initial content",
			ComposeCmd("build", "--build-arg", "CONTENT=foo")).
		Step("up mounts the image volume into the consumer",
			ComposeCmd("up", "-d")).
		Step("the consumer read the initial content",
			ComposeCmd("logs", "consumer"),
			OutputContains("foo")).
		Step("rebuild the source with new content",
			ComposeCmd("build", "--build-arg", "CONTENT=bar")).
		Step("up recreates the consumer against the rebuilt image",
			ComposeCmd("up", "-d"),
			Recreated("consumer")).
		Step("the consumer read the new content",
			ComposeCmd("logs", "consumer"),
			OutputContains("bar"))
}
