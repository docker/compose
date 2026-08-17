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
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"
)

// The publish checks all run with --dry-run: the CLI's decision — prompt,
// refusal or publication report — is the observable.

func TestPublishPromptEnvFile(t *testing.T) {
	NewScenario(t, "publish must prompt on env_file declarations, and --with-env must silence the prompt").
		Step("declining the env prompt aborts the publication",
			ComposeCmd("publish", "test/test", "--dry-run").WithStdin("n\n").MayFail(),
			ExitCode(130),
			OutputContains("you are about to publish env-related declarations within your OCI artifact."),
			OutputContains(`service "serviceA": env_file declared`),
			OutputContains("Are you ok to publish these env declarations?"),
			OutputNotContains("test/test published")).
		Step("--with-env publishes without prompting",
			ComposeCmd("publish", "test/test", "--with-env", "-y", "--dry-run"),
			OutputContains("test/test publishing"),
			OutputContains("test/test published"))
}

func TestPublishPromptSuspiciousEnv(t *testing.T) {
	NewScenario(t, "publish must prompt on suspicious env literals, and --with-env must override").
		Step("declining the prompt aborts the publication",
			ComposeCmd("publish", "test/test", "--dry-run").WithStdin("n\n").MayFail(),
			ExitCode(130),
			OutputContains(`service "serviceA": literal value for "MYSQL_ROOT_PASSWORD"`)).
		Step("--with-env publishes without prompting",
			ComposeCmd("publish", "test/test", "--with-env", "-y", "--dry-run"),
			OutputContains("test/test publishing"),
			OutputContains("test/test published"))
}

func TestPublishInterpolatedEnv(t *testing.T) {
	NewScenario(t, "interpolated env values must publish without an env prompt").
		Step("publish succeeds with -y only",
			ComposeCmd("publish", "test/test", "-y", "--dry-run"),
			OutputContains("test/test publishing"),
			OutputContains("test/test published"))
}

func TestPublishPromptAggregatesFindings(t *testing.T) {
	NewScenario(t, "the env prompt must aggregate env_file and suspicious literals across services").
		Step("every finding is listed before the prompt",
			ComposeCmd("publish", "test/test", "--dry-run").WithStdin("n\n").MayFail(),
			ExitCode(130),
			OutputContains(`service "serviceB": env_file declared`),
			OutputContains(`service "serviceA": literal value for "DB_PASSWORD"`),
			OutputContains(`service "serviceB": literal value for "API_KEY"`),
			OutputContains("Use --with-env to silence this prompt"))
}

func TestPublishWithExtends(t *testing.T) {
	NewScenario(t, "a model using extends must publish, resolved").
		Step("publish succeeds",
			ComposeCmd("publish", "test/test", "--dry-run"),
			OutputContains("test/test published"))
}

func TestPublishBindMount(t *testing.T) {
	NewScenario(t, "publish must prompt on bind mounts, honoring the answer").
		Step("declining the bind-mount prompt aborts the publication",
			ComposeCmd("publish", "test/test", "--dry-run").WithStdin("n\n").MayFail(),
			ExitCode(130),
			OutputContains("you are about to publish bind mounts declaration within your OCI artifact."),
			OutputContains(":/user-data"),
			OutputContains("Are you ok to publish these bind mount declarations?"),
			OutputNotContains("test/test published")).
		Step("accepting the prompt publishes",
			ComposeCmd("publish", "test/test", "--dry-run").WithStdin("y\n"),
			OutputContains(":/user-data"),
			OutputContains("test/test published"))
}

func TestPublishBuildOnly(t *testing.T) {
	NewScenario(t, "a stack of build-only services must be refused").
		Step("publish is rejected, naming the build-only services",
			ComposeCmd("publish", "test/test", "--with-env", "-y", "--dry-run").MayFail(),
			ExitCode(1),
			OutputContains("your Compose stack cannot be published as it only contains a build section for service(s):"),
			OutputContains("serviceA"),
			OutputContains("serviceB"))
}

func TestPublishLocalInclude(t *testing.T) {
	NewScenario(t, "a model with a local include must be refused").
		Step("publish is rejected",
			ComposeCmd("publish", "test/test", "--dry-run").MayFail(),
			ExitCode(1),
			OutputContains("cannot publish compose file with local includes"))
}

func TestPublishDetectSensitiveData(t *testing.T) {
	NewScenario(t, "publish must detect and list sensitive data before prompting").
		Step("every category of sensitive data is reported",
			ComposeCmd("publish", "test/test", "--with-env", "--dry-run").WithStdin("n\n").MayFail(),
			ExitCode(130),
			OutputContains("you are about to publish sensitive data within your OCI artifact.\n"),
			OutputContains("please double check that you are not leaking sensitive data"),
			OutputContains("AWS Client ID\n\"services.serviceA.environment.AWS_ACCESS_KEY_ID\": A3TX1234567890ABCDEF"),
			OutputContains("AWS Secret Key\n\"services.serviceA.environment.AWS_SECRET_ACCESS_KEY\": aws\"12345+67890/abcdefghijklm+NOPQRSTUVWXYZ+\""),
			OutputContains("Github authentication\n\"GITHUB_TOKEN\": ghp_1234567890abcdefghijklmnopqrstuvwxyz"),
			OutputContains("JSON Web Token\n\"\": eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9."+
				"eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw"),
			OutputContains("Private Key\n\"\": -----BEGIN DSA PRIVATE KEY-----\nwxyz+ABC=\n-----END DSA PRIVATE KEY-----"))
}

func TestPublish(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-publish"
	const registryName = projectName + "-registry"
	c.RunDockerCmd(t, "run", "--name", registryName, "-P", "-d", "registry:3")
	port := c.RunDockerCmd(t, "inspect", "--format", `{{ (index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort }}`, registryName).Stdout()
	registry := "localhost:" + strings.TrimSpace(port)
	t.Cleanup(func() {
		c.RunDockerCmd(t, "rm", "--force", registryName)
	})

	// Wait for registry to be ready
	registryURL := "http://" + registry + "/v2/"
	poll.WaitOn(t, func(l poll.LogT) poll.Result {
		resp, err := http.Get(registryURL) //nolint:gosec,noctx
		if err != nil {
			return poll.Continue("registry not ready: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 500 {
			return poll.Success()
		}
		return poll.Continue("registry not ready, status %d", resp.StatusCode)
	}, poll.WithTimeout(10*time.Second), poll.WithDelay(100*time.Millisecond))

	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/publish/oci/compose.yaml", "-f", "./fixtures/publish/oci/compose-override.yaml",
		"-p", projectName, "publish", "--with-env", "--yes", "--insecure-registry", registry+"/test:test")
	res.Assert(t, icmd.Expected{ExitCode: 0})

	// docker exec -it compose-e2e-publish-registry tree /var/lib/registry/docker/registry/v2/

	cmd := c.NewDockerComposeCmd(t, "--verbose", "--project-name=oci",
		"--insecure-registry", registry,
		"-f", fmt.Sprintf("oci://%s/test:test", registry), "config")
	res = icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
		cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+t.TempDir())
	})
	res.Assert(t, icmd.Expected{ExitCode: 0})
	assert.Equal(t, res.Stdout(), `name: oci
services:
  app:
    environment:
      HELLO: WORLD
    image: alpine
    networks:
      default: null
networks:
  default:
    name: oci_default
`)

	// `up` loads the project twice: once through ToProject, then again through
	// ToModel (checksForRemoteStack -> promptForInterpolatedVariables) to list
	// the interpolation variables when --yes is not passed. That second load
	// builds its own OCI loader, which used to drop --insecure-registry and so
	// talked HTTPS to a plain-HTTP registry. See docker/compose#13824.
	res = c.RunDockerComposeCmd(t, "-f", "./fixtures/publish/oci/compose-interpolated.yaml",
		"-p", projectName, "publish", "--with-env", "--yes", "--insecure-registry", registry+"/test:interpolated")
	res.Assert(t, icmd.Expected{ExitCode: 0})

	// Declining the prompt keeps the test hermetic: nothing is created, but the
	// re-load has already happened by then, which is what we want to cover.
	cmd = c.NewDockerComposeCmd(t, "--project-name=oci-reload",
		"--insecure-registry", registry,
		"-f", fmt.Sprintf("oci://%s/test:interpolated", registry), "up")
	cmd.Stdin = strings.NewReader("n\n")
	res = icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
		cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+t.TempDir())
	})
	assert.Assert(t, !strings.Contains(res.Combined(), "server gave HTTP response to HTTPS client"), res.Combined())
	res.Assert(t, icmd.Expected{ExitCode: 1, Err: "operation cancelled by user"})
}
