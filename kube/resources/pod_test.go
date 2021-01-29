// +build kube

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

package resources

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/stretchr/testify/assert"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func loadYAML(yaml string) (*types.Project, error) {
	dict, err := loader.ParseYAML([]byte(yaml))
	if err != nil {
		return nil, err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	configs := []types.ConfigFile{
		{
			Filename: "test-compose.yaml",
			Config:   dict,
		},
	}
	config := types.ConfigDetails{
		WorkingDir:  workingDir,
		ConfigFiles: configs,
		Environment: nil,
	}
	return loader.Load(config)
}

func podTemplate(t *testing.T, yaml string) apiv1.PodTemplateSpec {
	res, err := podTemplateWithError(yaml)
	assert.NoError(t, err)
	return res
}

func podTemplateWithError(yaml string) (apiv1.PodTemplateSpec, error) {
	model, err := loadYAML(yaml)
	if err != nil {
		return apiv1.PodTemplateSpec{}, err
	}

	return toPodTemplate(model, model.Services[0], nil)
}

func TestToPodWithDockerSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("on windows, source path validation is broken (and actually, source validation for windows workload is broken too). Skip it for now, as we don't support it yet")
		return
	}
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: "redis:alpine"
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock"
`)

	expectedVolume := apiv1.Volume{
		Name: "mount-0",
		VolumeSource: apiv1.VolumeSource{
			HostPath: &apiv1.HostPathVolumeSource{
				Path: "/var/run",
			},
		},
	}

	expectedMount := apiv1.VolumeMount{
		Name:      "mount-0",
		MountPath: "/var/run/docker.sock",
		SubPath:   "docker.sock",
	}

	assert.Len(t, podTemplate.Spec.Volumes, 1)
	assert.Len(t, podTemplate.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, expectedVolume, podTemplate.Spec.Volumes[0])
	assert.Equal(t, expectedMount, podTemplate.Spec.Containers[0].VolumeMounts[0])
}

func TestToPodWithFunkyCommand(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: basi/node-exporter
    command: ["-collector.procfs", "/host/proc", "-collector.sysfs", "/host/sys"]
`)

	expectedArgs := []string{
		`-collector.procfs`,
		`/host/proc`, // ?
		`-collector.sysfs`,
		`/host/sys`, // ?
	}
	assert.Equal(t, expectedArgs, podTemplate.Spec.Containers[0].Args)
}

/* FIXME
func TestToPodWithGlobalVolume(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  db:
    image: "postgres:9.4"
    volumes:
	  - dbdata:/var/lib/postgresql/data
volumes:
  dbdata:
`)

	expectedMount := apiv1.VolumeMount{
		Name:      "dbdata",
		MountPath: "/var/lib/postgresql/data",
	}
	assert.Len(t, podTemplate.Spec.Volumes, 0)
	assert.Len(t, podTemplate.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, expectedMount, podTemplate.Spec.Containers[0].VolumeMounts[0])
}
*/

func TestToPodWithResources(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  db:
    image: "postgres:9.4"
    deploy:
      resources:
        limits:
          cpus: "0.001"
          memory: 50Mb
        reservations:
          cpus: "0.0001"
          memory: 20Mb
`)

	expectedResourceRequirements := apiv1.ResourceRequirements{
		Limits: map[apiv1.ResourceName]resource.Quantity{
			apiv1.ResourceCPU:    resource.MustParse("0.001"),
			apiv1.ResourceMemory: resource.MustParse(fmt.Sprintf("%d", 50*1024*1024)),
		},
		Requests: map[apiv1.ResourceName]resource.Quantity{
			apiv1.ResourceCPU:    resource.MustParse("0.0001"),
			apiv1.ResourceMemory: resource.MustParse(fmt.Sprintf("%d", 20*1024*1024)),
		},
	}
	assert.Equal(t, expectedResourceRequirements, podTemplate.Spec.Containers[0].Resources)
}

func TestToPodWithCapabilities(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: "redis:alpine"
    cap_add:
      - ALL
    cap_drop:
      - NET_ADMIN
      - SYS_ADMIN
`)

	expectedSecurityContext := &apiv1.SecurityContext{
		Capabilities: &apiv1.Capabilities{
			Add:  []apiv1.Capability{"ALL"},
			Drop: []apiv1.Capability{"NET_ADMIN", "SYS_ADMIN"},
		},
	}

	assert.Equal(t, expectedSecurityContext, podTemplate.Spec.Containers[0].SecurityContext)
}

func TestToPodWithReadOnly(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: "redis:alpine"
    read_only: true
`)

	yes := true
	expectedSecurityContext := &apiv1.SecurityContext{
		ReadOnlyRootFilesystem: &yes,
	}
	assert.Equal(t, expectedSecurityContext, podTemplate.Spec.Containers[0].SecurityContext)
}

func TestToPodWithPrivileged(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: "redis:alpine"
    privileged: true
`)

	yes := true
	expectedSecurityContext := &apiv1.SecurityContext{
		Privileged: &yes,
	}
	assert.Equal(t, expectedSecurityContext, podTemplate.Spec.Containers[0].SecurityContext)
}

func TestToPodWithEnvNilShouldErrorOut(t *testing.T) {
	_, err := podTemplateWithError(`
version: "3"
services:
  redis:
    image: "redis:alpine"
    environment:
      - SESSION_SECRET
`)
	assert.Error(t, err)
}

func TestToPodWithEnv(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: "redis:alpine"
    environment:
      - RACK_ENV=development
      - SHOW=true
`)

	expectedEnv := []apiv1.EnvVar{
		{
			Name:  "RACK_ENV",
			Value: "development",
		},
		{
			Name:  "SHOW",
			Value: "true",
		},
	}

	assert.Equal(t, expectedEnv, podTemplate.Spec.Containers[0].Env)
}

func TestToPodWithVolume(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("on windows, source path validation is broken (and actually, source validation for windows workload is broken too). Skip it for now, as we don't support it yet")
		return
	}
	podTemplate := podTemplate(t, `
version: "3"
services:
  nginx:
    image: nginx
    volumes:
      - /ignore:/ignore
      - /opt/data:/var/lib/mysql:ro
`)

	assert.Len(t, podTemplate.Spec.Volumes, 2)
	assert.Len(t, podTemplate.Spec.Containers[0].VolumeMounts, 2)
}

/* FIXME
func TestToPodWithRelativeVolumes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("on windows, source path validation is broken (and actually, source validation for windows workload is broken too). Skip it for now, as we don't support it yet")
		return
	}
	_, err := podTemplateWithError(`
version: "3"
services:
  nginx:
    image: nginx
    volumes:
      - ./fail:/ignore
`)

	assert.Error(t, err)
}
*/

func TestToPodWithHealthCheck(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  nginx:
    image: nginx
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost"]
      interval: 90s
      timeout: 10s
      retries: 3
`)

	expectedLivenessProbe := &apiv1.Probe{
		TimeoutSeconds:   10,
		PeriodSeconds:    90,
		FailureThreshold: 3,
		Handler: apiv1.Handler{
			Exec: &apiv1.ExecAction{
				Command: []string{"curl", "-f", "http://localhost"},
			},
		},
	}

	assert.Equal(t, expectedLivenessProbe, podTemplate.Spec.Containers[0].LivenessProbe)
}

func TestToPodWithShellHealthCheck(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  nginx:
    image: nginx
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost"]
`)

	expectedLivenessProbe := &apiv1.Probe{
		TimeoutSeconds:   1,
		PeriodSeconds:    1,
		FailureThreshold: 3,
		Handler: apiv1.Handler{
			Exec: &apiv1.ExecAction{
				Command: []string{"sh", "-c", "curl -f http://localhost"},
			},
		},
	}

	assert.Equal(t, expectedLivenessProbe, podTemplate.Spec.Containers[0].LivenessProbe)
}

/* FIXME
func TestToPodWithTargetlessExternalSecret(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  nginx:
    image: nginx
    secrets:
      - my_secret
`)

	expectedVolume := apiv1.Volume{
		Name: "secret-0",
		VolumeSource: apiv1.VolumeSource{
			Secret: &apiv1.SecretVolumeSource{
				SecretName: "my_secret",
				Items: []apiv1.KeyToPath{
					{
						Key:  "file", // TODO: This is the key we assume external secrets use
						Path: "secret-0",
					},
				},
			},
		},
	}

	expectedMount := apiv1.VolumeMount{
		Name:      "secret-0",
		ReadOnly:  true,
		MountPath: "/run/secrets/my_secret",
		SubPath:   "secret-0",
	}

	assert.Len(t, podTemplate.Spec.Volumes, 1)
	assert.Len(t, podTemplate.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, expectedVolume, podTemplate.Spec.Volumes[0])
	assert.Equal(t, expectedMount, podTemplate.Spec.Containers[0].VolumeMounts[0])
}
*/

/* FIXME
func TestToPodWithExternalSecret(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  nginx:
    image: nginx
    secrets:
      - source: my_secret
        target: nginx_secret
`)

	expectedVolume := apiv1.Volume{
		Name: "secret-0",
		VolumeSource: apiv1.VolumeSource{
			Secret: &apiv1.SecretVolumeSource{
				SecretName: "my_secret",
				Items: []apiv1.KeyToPath{
					{
						Key:  "file", // TODO: This is the key we assume external secrets use
						Path: "secret-0",
					},
				},
			},
		},
	}

	expectedMount := apiv1.VolumeMount{
		Name:      "secret-0",
		ReadOnly:  true,
		MountPath: "/run/secrets/nginx_secret",
		SubPath:   "secret-0",
	}

	assert.Len(t, podTemplate.Spec.Volumes, 1)
	assert.Len(t, podTemplate.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, expectedVolume, podTemplate.Spec.Volumes[0])
	assert.Equal(t, expectedMount, podTemplate.Spec.Containers[0].VolumeMounts[0])
}
*/

/* FIXME
func TestToPodWithFileBasedSecret(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  nginx:
    image: nginx
    secrets:
      - source: my_secret
secrets:
  my_secret:
    file: ./secret.txt
`)

	expectedVolume := apiv1.Volume{
		Name: "secret-0",
		VolumeSource: apiv1.VolumeSource{
			Secret: &apiv1.SecretVolumeSource{
				SecretName: "my_secret",
				Items: []apiv1.KeyToPath{
					{
						Key:  "secret.txt",
						Path: "secret-0",
					},
				},
			},
		},
	}

	expectedMount := apiv1.VolumeMount{
		Name:      "secret-0",
		ReadOnly:  true,
		MountPath: "/run/secrets/my_secret",
		SubPath:   "secret-0",
	}

	assert.Len(t, podTemplate.Spec.Volumes, 1)
	assert.Len(t, podTemplate.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, expectedVolume, podTemplate.Spec.Volumes[0])
	assert.Equal(t, expectedMount, podTemplate.Spec.Containers[0].VolumeMounts[0])
}
*/

/* FIXME
func TestToPodWithTwoFileBasedSecrets(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  nginx:
    image: nginx
    secrets:
      - source: my_secret1
      - source: my_secret2
        target: secret2
secrets:
  my_secret1:
    file: ./secret1.txt
  my_secret2:
    file: ./secret2.txt
`)

	expectedVolumes := []apiv1.Volume{
		{
			Name: "secret-0",
			VolumeSource: apiv1.VolumeSource{
				Secret: &apiv1.SecretVolumeSource{
					SecretName: "my_secret1",
					Items: []apiv1.KeyToPath{
						{
							Key:  "secret1.txt",
							Path: "secret-0",
						},
					},
				},
			},
		},
		{
			Name: "secret-1",
			VolumeSource: apiv1.VolumeSource{
				Secret: &apiv1.SecretVolumeSource{
					SecretName: "my_secret2",
					Items: []apiv1.KeyToPath{
						{
							Key:  "secret2.txt",
							Path: "secret-1",
						},
					},
				},
			},
		},
	}

	expectedMounts := []apiv1.VolumeMount{
		{
			Name:      "secret-0",
			ReadOnly:  true,
			MountPath: "/run/secrets/my_secret1",
			SubPath:   "secret-0",
		},
		{
			Name:      "secret-1",
			ReadOnly:  true,
			MountPath: "/run/secrets/secret2",
			SubPath:   "secret-1",
		},
	}

	assert.Equal(t, expectedVolumes, podTemplate.Spec.Volumes)
	assert.Equal(t, expectedMounts, podTemplate.Spec.Containers[0].VolumeMounts)
}
*/

func TestToPodWithTerminationGracePeriod(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: "redis:alpine"
    stop_grace_period: 100s
`)

	expected := int64(100)
	assert.Equal(t, &expected, podTemplate.Spec.TerminationGracePeriodSeconds)
}

func TestToPodWithTmpfs(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: "redis:alpine"
    tmpfs:
      - /tmp
`)

	expectedVolume := apiv1.Volume{
		Name: "tmp-0",
		VolumeSource: apiv1.VolumeSource{
			EmptyDir: &apiv1.EmptyDirVolumeSource{
				Medium: "Memory",
			},
		},
	}

	expectedMount := apiv1.VolumeMount{
		Name:      "tmp-0",
		MountPath: "/tmp",
	}

	assert.Len(t, podTemplate.Spec.Volumes, 1)
	assert.Len(t, podTemplate.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, expectedVolume, podTemplate.Spec.Volumes[0])
	assert.Equal(t, expectedMount, podTemplate.Spec.Containers[0].VolumeMounts[0])
}

func TestToPodWithNumericalUser(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: "redis:alpine"
    user: "1000"
`)

	userID := int64(1000)

	expectedSecurityContext := &apiv1.SecurityContext{
		RunAsUser: &userID,
	}

	assert.Equal(t, expectedSecurityContext, podTemplate.Spec.Containers[0].SecurityContext)
}

func TestToPodWithGitVolume(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: "redis:alpine"
    volumes:
      - source: "git@github.com:moby/moby.git"
        target: /sources
        type: git
`)

	expectedVolume := apiv1.Volume{
		Name: "mount-0",
		VolumeSource: apiv1.VolumeSource{
			GitRepo: &apiv1.GitRepoVolumeSource{
				Repository: "git@github.com:moby/moby.git",
			},
		},
	}

	expectedMount := apiv1.VolumeMount{
		Name:      "mount-0",
		ReadOnly:  false,
		MountPath: "/sources",
	}

	assert.Len(t, podTemplate.Spec.Volumes, 1)
	assert.Len(t, podTemplate.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, expectedVolume, podTemplate.Spec.Volumes[0])
	assert.Equal(t, expectedMount, podTemplate.Spec.Containers[0].VolumeMounts[0])
}

/* FIXME
func TestToPodWithFileBasedConfig(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
 redis:
    image: "redis:alpine"
    configs:
      - source: my_config
        target: /usr/share/nginx/html/index.html
        uid: "103"
        gid: "103"
        mode: 0440
configs:
  my_config:
    file: ./file.html
`)

	mode := int32(0440)

	expectedVolume := apiv1.Volume{
		Name: "config-0",
		VolumeSource: apiv1.VolumeSource{
			ConfigMap: &apiv1.ConfigMapVolumeSource{
				LocalObjectReference: apiv1.LocalObjectReference{
					Name: "my_config",
				},
				Items: []apiv1.KeyToPath{
					{
						Key:  "file.html",
						Path: "config-0",
						Mode: &mode,
					},
				},
			},
		},
	}

	expectedMount := apiv1.VolumeMount{
		Name:      "config-0",
		ReadOnly:  true,
		MountPath: "/usr/share/nginx/html/index.html",
		SubPath:   "config-0",
	}

	assert.Len(t, podTemplate.Spec.Volumes, 1)
	assert.Len(t, podTemplate.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, expectedVolume, podTemplate.Spec.Volumes[0])
	assert.Equal(t, expectedMount, podTemplate.Spec.Containers[0].VolumeMounts[0])
}
*/

/* FIXME
func TestToPodWithTargetlessFileBasedConfig(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: "redis:alpine"
    configs:
      - my_config
configs:
  my_config:
    file: ./file.html
`)

	expectedVolume := apiv1.Volume{
		Name: "config-0",
		VolumeSource: apiv1.VolumeSource{
			ConfigMap: &apiv1.ConfigMapVolumeSource{
				LocalObjectReference: apiv1.LocalObjectReference{
					Name: "myconfig",
				},
				Items: []apiv1.KeyToPath{
					{
						Key:  "file.html",
						Path: "config-0",
					},
				},
			},
		},
	}

	expectedMount := apiv1.VolumeMount{
		Name:      "config-0",
		ReadOnly:  true,
		MountPath: "/myconfig",
		SubPath:   "config-0",
	}

	assert.Len(t, podTemplate.Spec.Volumes, 1)
	assert.Len(t, podTemplate.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, expectedVolume, podTemplate.Spec.Volumes[0])
	assert.Equal(t, expectedMount, podTemplate.Spec.Containers[0].VolumeMounts[0])
}
*/

func TestToPodWithExternalConfig(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  redis:
    image: "redis:alpine"
    configs:
      - source: my_config
        target: /usr/share/nginx/html/index.html
        uid: "103"
        gid: "103"
        mode: 0440
configs:
  my_config:
    external: true
`)

	mode := int32(0440)

	expectedVolume := apiv1.Volume{
		Name: "config-0",
		VolumeSource: apiv1.VolumeSource{
			ConfigMap: &apiv1.ConfigMapVolumeSource{
				LocalObjectReference: apiv1.LocalObjectReference{
					Name: "my_config",
				},
				Items: []apiv1.KeyToPath{
					{
						Key:  "file", // TODO: This is the key we assume external config use
						Path: "config-0",
						Mode: &mode,
					},
				},
			},
		},
	}

	expectedMount := apiv1.VolumeMount{
		Name:      "config-0",
		ReadOnly:  true,
		MountPath: "/usr/share/nginx/html/index.html",
		SubPath:   "config-0",
	}

	assert.Len(t, podTemplate.Spec.Volumes, 1)
	assert.Len(t, podTemplate.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, expectedVolume, podTemplate.Spec.Volumes[0])
	assert.Equal(t, expectedMount, podTemplate.Spec.Containers[0].VolumeMounts[0])
}

/* FIXME
func TestToPodWithTwoConfigsSameMountPoint(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  nginx:
    image: nginx
    configs:
      - source: first
        target: /data/first.json
        mode: "0440"
      - source: second
        target: /data/second.json
        mode: "0550"
configs:
  first:
    file: ./file1
  secondv:
    file: ./file2
`)

	mode0440 := int32(0440)
	mode0550 := int32(0550)

	expectedVolumes := []apiv1.Volume{
		{
			Name: "config-0",
			VolumeSource: apiv1.VolumeSource{
				ConfigMap: &apiv1.ConfigMapVolumeSource{
					LocalObjectReference: apiv1.LocalObjectReference{
						Name: "first",
					},
					Items: []apiv1.KeyToPath{
						{
							Key:  "file1",
							Path: "config-0",
							Mode: &mode0440,
						},
					},
				},
			},
		},
		{
			Name: "config-1",
			VolumeSource: apiv1.VolumeSource{
				ConfigMap: &apiv1.ConfigMapVolumeSource{
					LocalObjectReference: apiv1.LocalObjectReference{
						Name: "second",
					},
					Items: []apiv1.KeyToPath{
						{
							Key:  "file2",
							Path: "config-1",
							Mode: &mode0550,
						},
					},
				},
			},
		},
	}

	expectedMounts := []apiv1.VolumeMount{
		{
			Name:      "config-0",
			ReadOnly:  true,
			MountPath: "/data/first.json",
			SubPath:   "config-0",
		},
		{
			Name:      "config-1",
			ReadOnly:  true,
			MountPath: "/data/second.json",
			SubPath:   "config-1",
		},
	}

	assert.Equal(t, expectedVolumes, podTemplate.Spec.Volumes)
	assert.Equal(t, expectedMounts, podTemplate.Spec.Containers[0].VolumeMounts)
}
*/

func TestToPodWithTwoExternalConfigsSameMountPoint(t *testing.T) {
	podTemplate := podTemplate(t, `
version: "3"
services:
  nginx:
    image: nginx
    configs:
      - source: first
        target: /data/first.json
      - source: second
        target: /data/second.json
configs:
  first:
    file: ./file1
  second:
    file: ./file2
`)

	expectedVolumes := []apiv1.Volume{
		{
			Name: "config-0",
			VolumeSource: apiv1.VolumeSource{
				ConfigMap: &apiv1.ConfigMapVolumeSource{
					LocalObjectReference: apiv1.LocalObjectReference{
						Name: "first",
					},
					Items: []apiv1.KeyToPath{
						{
							Key:  "file",
							Path: "config-0",
						},
					},
				},
			},
		},
		{
			Name: "config-1",
			VolumeSource: apiv1.VolumeSource{
				ConfigMap: &apiv1.ConfigMapVolumeSource{
					LocalObjectReference: apiv1.LocalObjectReference{
						Name: "second",
					},
					Items: []apiv1.KeyToPath{
						{
							Key:  "file",
							Path: "config-1",
						},
					},
				},
			},
		},
	}

	expectedMounts := []apiv1.VolumeMount{
		{
			Name:      "config-0",
			ReadOnly:  true,
			MountPath: "/data/first.json",
			SubPath:   "config-0",
		},
		{
			Name:      "config-1",
			ReadOnly:  true,
			MountPath: "/data/second.json",
			SubPath:   "config-1",
		},
	}

	assert.Equal(t, expectedVolumes, podTemplate.Spec.Volumes)
	assert.Equal(t, expectedMounts, podTemplate.Spec.Containers[0].VolumeMounts)
}

/* FIXME
func TestToPodWithPullSecret(t *testing.T) {
	podTemplateWithSecret := podTemplate(t, `
version: "3"
services:
  nginx:
    image: nginx
    x-kubernetes.pull-secret: test-pull-secret
`)

	assert.Equal(t, 1, len(podTemplateWithSecret.Spec.ImagePullSecrets))
	assert.Equal(t, "test-pull-secret", podTemplateWithSecret.Spec.ImagePullSecrets[0].Name)

	podTemplateNoSecret := podTemplate(t, `
version: "3"
services:
  nginx:
    image: nginx
`)

	assert.Nil(t, podTemplateNoSecret.Spec.ImagePullSecrets)
}
*/

/* FIXME
func TestToPodWithPullPolicy(t *testing.T) {
	cases := []struct {
		name           string
		stack          string
		expectedPolicy apiv1.PullPolicy
		expectedError  string
	}{
		{
			name: "specific tag",
			stack: `
version: "3"
services:
  nginx:
    image: nginx:specific
`,
			expectedPolicy: apiv1.PullIfNotPresent,
		},
		{
			name: "latest tag",
			stack: `
version: "3"
services:
  nginx:
    image: nginx:latest
`,
			expectedPolicy: apiv1.PullAlways,
		},
		{
			name: "explicit policy",
			stack: `
version: "3"
services:
  nginx:
    image: nginx:specific
    x-kubernetes.pull-policy: Never
`,
			expectedPolicy: apiv1.PullNever,
		},
		{
			name: "invalid policy",
			stack: `
version: "3"
services:
  nginx:
    image: nginx:specific
    x-kubernetes.pull-policy: Invalid
`,
			expectedError: `invalid pull policy "Invalid", must be "Always", "IfNotPresent" or "Never"`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pod, err := podTemplateWithError(c.stack)
			if c.expectedError != "" {
				assert.EqualError(t, err, c.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, pod.Spec.Containers[0].ImagePullPolicy, c.expectedPolicy)
			}
		})
	}
}
*/
