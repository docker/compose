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

package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/resources/mgmt/resources"
	azure_storage "github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/storage/mgmt/storage"
	"github.com/Azure/azure-storage-file-go/azfile"
	"github.com/Azure/go-autorest/autorest/to"

	"github.com/docker/api/aci"
	"github.com/docker/api/aci/login"
	"github.com/docker/api/containers"
	"github.com/docker/api/context/store"
	"github.com/docker/api/errdefs"
	"github.com/docker/api/tests/aci-e2e/storage"
	. "github.com/docker/api/tests/framework"
)

const (
	contextName = "aci-test"
	location    = "westeurope"
)

var binDir string

func TestMain(m *testing.M) {
	p, cleanup, err := SetupExistingCLI()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	binDir = p
	exitCode := m.Run()
	cleanup()
	os.Exit(exitCode)
}

// Cannot be parallelized as login/logout is global.
func TestLoginLogout(t *testing.T) {
	startTime := strconv.Itoa(int(time.Now().UnixNano()))
	c := NewE2eCLI(t, binDir)
	rg := "E2E-" + startTime

	t.Run("login", func(t *testing.T) {
		azureLogin(t, c)
	})

	t.Run("create context", func(t *testing.T) {
		sID := getSubscriptionID(t)
		err := createResourceGroup(sID, rg)
		assert.Check(t, is.Nil(err))
		t.Cleanup(func() {
			_ = deleteResourceGroup(rg)
		})

		res := c.RunDockerCmd("context", "create", "aci", contextName, "--subscription-id", sID, "--resource-group", rg, "--location", location)
		res.Assert(t, icmd.Success)
		res = c.RunDockerCmd("context", "use", contextName)
		res.Assert(t, icmd.Expected{Out: contextName})
		res = c.RunDockerCmd("context", "ls")
		res.Assert(t, icmd.Expected{Out: contextName + " *"})
	})

	t.Run("delete context", func(t *testing.T) {
		res := c.RunDockerCmd("context", "use", "default")
		res.Assert(t, icmd.Expected{Out: "default"})

		res = c.RunDockerCmd("context", "rm", contextName)
		res.Assert(t, icmd.Expected{Out: contextName})
	})

	t.Run("logout", func(t *testing.T) {
		_, err := os.Stat(login.GetTokenStorePath())
		assert.NilError(t, err)
		res := c.RunDockerCmd("logout", "azure")
		res.Assert(t, icmd.Expected{Out: "Removing login credentials for Azure"})
		_, err = os.Stat(login.GetTokenStorePath())
		errMsg := "no such file or directory"
		if runtime.GOOS == "windows" {
			errMsg = "The system cannot find the file specified"
		}
		assert.ErrorContains(t, err, errMsg)
	})

	t.Run("create context fail", func(t *testing.T) {
		res := c.RunDockerCmd("context", "create", "aci", "fail-context")
		res.Assert(t, icmd.Expected{
			ExitCode: errdefs.ExitCodeLoginRequired,
			Err:      `not logged in to azure, you need to run "docker login azure" first`,
		})
	})
}

func TestContainerRun(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	sID, rg := setupTestResourceGroup(t, c, "run")

	const (
		testShareName   = "dockertestshare"
		testFileContent = "Volume mounted successfully!"
		testFileName    = "index.html"
	)

	// Bootstrap volume
	aciContext := store.AciContext{
		SubscriptionID: sID,
		Location:       location,
		ResourceGroup:  rg,
	}
	saName := "e2e" + strconv.Itoa(int(time.Now().UnixNano()))
	_, cleanupSa := createStorageAccount(t, aciContext, saName)
	t.Cleanup(func() {
		if err := cleanupSa(); err != nil {
			t.Error(err)
		}
	})
	keys := getStorageKeys(t, aciContext, saName)
	assert.Assert(t, len(keys) > 0)
	k := *keys[0].Value
	cred, u := createFileShare(t, k, testShareName, saName)
	uploadFile(t, *cred, u.String(), testFileName, testFileContent)

	// Used in subtests
	var (
		container string
		hostIP    string
		endpoint  string
	)

	t.Run("run", func(t *testing.T) {
		mountTarget := "/usr/share/nginx/html"
		res := c.RunDockerCmd(
			"run", "-d",
			"-v", fmt.Sprintf("%s:%s@%s:%s", saName, k, testShareName, mountTarget),
			"-p", "80:80",
			"nginx",
		)
		res.Assert(t, icmd.Success)
		container = getContainerName(res.Stdout())
		t.Logf("Container name: %q", container)
	})

	t.Run("inspect", func(t *testing.T) {
		res := c.RunDockerCmd("inspect", container)
		res.Assert(t, icmd.Success)

		containerInspect, err := ParseContainerInspect(res.Stdout())
		assert.NilError(t, err)
		assert.Equal(t, containerInspect.Platform, "Linux")
		assert.Equal(t, containerInspect.CPULimit, 1.0)
		assert.Equal(t, containerInspect.RestartPolicyCondition, containers.RestartPolicyNone)

		assert.Assert(t, is.Len(containerInspect.Ports, 1))
		hostIP = containerInspect.Ports[0].HostIP
		endpoint = fmt.Sprintf("http://%s:%d", containerInspect.Ports[0].HostIP, containerInspect.Ports[0].HostPort)
		t.Logf("Endpoint: %s", endpoint)
	})

	t.Run("ps", func(t *testing.T) {
		res := c.RunDockerCmd("ps")
		res.Assert(t, icmd.Success)
		out := strings.Split(strings.TrimSpace(res.Stdout()), "\n")
		l := out[len(out)-1]
		assert.Assert(t, strings.Contains(l, container), "Looking for %q in line: %s", container, l)
		assert.Assert(t, strings.Contains(l, "nginx"))
		assert.Assert(t, strings.Contains(l, "Running"))
		assert.Assert(t, strings.Contains(l, hostIP+":80->80/tcp"))
	})

	t.Run("http get", func(t *testing.T) {
		r, err := http.Get(endpoint)
		assert.NilError(t, err)
		assert.Equal(t, r.StatusCode, http.StatusOK)
		b, err := ioutil.ReadAll(r.Body)
		assert.NilError(t, err)
		assert.Assert(t, strings.Contains(string(b), testFileContent), "Actual content: "+string(b))
	})

	t.Run("logs", func(t *testing.T) {
		res := c.RunDockerCmd("logs", container)
		res.Assert(t, icmd.Expected{Out: "GET"})
	})

	t.Run("exec", func(t *testing.T) {
		res := c.RunDockerCmd("exec", container, "pwd")
		res.Assert(t, icmd.Expected{Out: "/"})

		res = c.RunDockerCmd("exec", container, "echo", "fail_with_argument")
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "ACI exec command does not accept arguments to the command. Only the binary should be specified",
		})
	})

	t.Run("logs follow", func(t *testing.T) {
		cmd := c.NewDockerCmd("logs", "--follow", container)
		res := icmd.StartCmd(cmd)

		checkUp := func(t poll.LogT) poll.Result {
			r, _ := http.Get(endpoint + "/is_up")
			if r != nil && r.StatusCode == http.StatusNotFound {
				return poll.Success()
			}
			return poll.Continue("waiting for container to serve request")
		}
		poll.WaitOn(t, checkUp, poll.WithDelay(1*time.Second), poll.WithTimeout(60*time.Second))

		assert.Assert(t, !strings.Contains(res.Stdout(), "/test"))

		checkLogs := func(t poll.LogT) poll.Result {
			if strings.Contains(res.Stdout(), "/test") {
				return poll.Success()
			}
			return poll.Continue("waiting for logs to contain /test")
		}

		// Do request on /test
		go func() {
			time.Sleep(3 * time.Second)
			_, _ = http.Get(endpoint + "/test")
		}()

		poll.WaitOn(t, checkLogs, poll.WithDelay(3*time.Second), poll.WithTimeout(20*time.Second))

		if runtime.GOOS == "windows" {
			err := res.Cmd.Process.Kill()
			assert.NilError(t, err)
		} else {
			err := res.Cmd.Process.Signal(syscall.SIGTERM)
			assert.NilError(t, err)
		}
	})

	t.Run("rm a running container", func(t *testing.T) {
		res := c.RunDockerCmd("rm", container)
		res.Assert(t, icmd.Expected{
			Err:      fmt.Sprintf("Error: you cannot remove a running container %s. Stop the container before attempting removal or force remove", container),
			ExitCode: 1,
		})
	})

	t.Run("force rm", func(t *testing.T) {
		res := c.RunDockerCmd("rm", "-f", container)
		res.Assert(t, icmd.Expected{
			Out:      container,
			ExitCode: 0,
		})

		checkStopped := func(t poll.LogT) poll.Result {
			res := c.RunDockerCmd("inspect", container)
			if res.ExitCode == 1 {
				return poll.Success()
			}
			return poll.Continue("waiting for container to stop")
		}
		poll.WaitOn(t, checkStopped, poll.WithDelay(5*time.Second), poll.WithTimeout(60*time.Second))
	})
}

func TestContainerRunAttached(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	_, _ = setupTestResourceGroup(t, c, "runA")

	// Used in subtests
	var (
		container string
		endpoint  string
	)

	container = "test-container"

	t.Run("run attached limits", func(t *testing.T) {
		cmd := c.NewDockerCmd(
			"run",
			"--name", container,
			"--restart", "on-failure",
			"--memory", "0.1G", "--cpus", "0.1",
			"-p", "80:80",
			"nginx",
		)
		runRes := icmd.StartCmd(cmd)

		checkRunning := func(t poll.LogT) poll.Result {
			res := c.RunDockerCmd("inspect", container)
			if res.ExitCode == 0 {
				return poll.Success()
			}
			return poll.Continue("waiting for container to be running")
		}
		poll.WaitOn(t, checkRunning, poll.WithDelay(5*time.Second), poll.WithTimeout(60*time.Second))

		inspectRes := c.RunDockerCmd("inspect", container)
		inspectRes.Assert(t, icmd.Success)

		containerInspect, err := ParseContainerInspect(inspectRes.Stdout())
		assert.NilError(t, err)
		assert.Equal(t, containerInspect.Platform, "Linux")
		assert.Equal(t, containerInspect.CPULimit, 0.1)
		assert.Equal(t, containerInspect.MemoryLimit, uint64(107374182))
		assert.Equal(t, containerInspect.RestartPolicyCondition, containers.RestartPolicyOnFailure)

		assert.Assert(t, is.Len(containerInspect.Ports, 1))
		port := containerInspect.Ports[0]
		assert.Assert(t, len(port.HostIP) > 0)
		assert.Equal(t, port.ContainerPort, uint32(80))
		assert.Equal(t, port.HostPort, uint32(80))
		endpoint = fmt.Sprintf("http://%s:%d", port.HostIP, port.HostPort)
		t.Logf("Endpoint: %s", endpoint)

		assert.Assert(t, !strings.Contains(runRes.Stdout(), "/test"))
		checkRequest := func(t poll.LogT) poll.Result {
			r, _ := http.Get(endpoint + "/test")
			if r != nil && r.StatusCode == http.StatusNotFound {
				return poll.Success()
			}
			return poll.Continue("waiting for container to serve request")
		}
		poll.WaitOn(t, checkRequest, poll.WithDelay(1*time.Second), poll.WithTimeout(60*time.Second))

		checkLog := func(t poll.LogT) poll.Result {
			if strings.Contains(runRes.Stdout(), "/test") {
				return poll.Success()
			}
			return poll.Continue("waiting for logs to contain /test")
		}
		poll.WaitOn(t, checkLog, poll.WithDelay(1*time.Second), poll.WithTimeout(20*time.Second))
	})

	t.Run("stop wrong container", func(t *testing.T) {
		res := c.RunDockerCmd("stop", "unknown-container")
		res.Assert(t, icmd.Expected{
			Err:      "Error: container unknown-container not found",
			ExitCode: 1,
		})
	})

	t.Run("stop container", func(t *testing.T) {
		res := c.RunDockerCmd("stop", container)
		res.Assert(t, icmd.Expected{Out: container})
	})

	t.Run("rm stopped container", func(t *testing.T) {
		res := c.RunDockerCmd("rm", container)
		res.Assert(t, icmd.Expected{Out: container})
	})
}

func TestCompose(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	_, _ = setupTestResourceGroup(t, c, "compose")

	const (
		composeFile              = "../composefiles/aci-demo/aci_demo_port.yaml"
		composeFileMultiplePorts = "../composefiles/aci-demo/aci_demo_multi_port.yaml"
		composeProjectName       = "acie2e"
		serverContainer          = composeProjectName + "_web"
		wordsContainer           = composeProjectName + "_words"
	)

	t.Run("compose up", func(t *testing.T) {
		// Name of Compose project is taken from current folder "acie2e"
		res := c.RunDockerCmd("compose", "up", "-f", composeFile)
		res.Assert(t, icmd.Success)

		res = c.RunDockerCmd("ps")
		res.Assert(t, icmd.Success)
		out := strings.Split(strings.TrimSpace(res.Stdout()), "\n")
		// Check three containers are running
		assert.Assert(t, is.Len(out, 4))
		webRunning := false
		for _, l := range out {
			if strings.Contains(l, serverContainer) {
				webRunning = true
				strings.Contains(l, ":80->80/tcp")
			}
		}
		assert.Assert(t, webRunning, "web container not running")

		res = c.RunDockerCmd("inspect", serverContainer)
		res.Assert(t, icmd.Success)

		containerInspect, err := ParseContainerInspect(res.Stdout())
		assert.NilError(t, err)
		assert.Assert(t, is.Len(containerInspect.Ports, 1))
		endpoint := fmt.Sprintf("http://%s:%d", containerInspect.Ports[0].HostIP, containerInspect.Ports[0].HostPort)
		t.Logf("Endpoint: %s", endpoint)

		r, err := http.Get(endpoint + "/words/noun")
		assert.NilError(t, err)
		assert.Equal(t, r.StatusCode, http.StatusOK)
		b, err := ioutil.ReadAll(r.Body)
		assert.NilError(t, err)
		assert.Assert(t, strings.Contains(string(b), `"word":`))
	})

	t.Run("logs web", func(t *testing.T) {
		res := c.RunDockerCmd("logs", serverContainer)
		res.Assert(t, icmd.Expected{Out: "Listening on port 80"})
	})

	t.Run("update", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "up", "-f", composeFileMultiplePorts, "--project-name", composeProjectName)
		res.Assert(t, icmd.Success)

		res = c.RunDockerCmd("ps")
		res.Assert(t, icmd.Success)
		out := strings.Split(strings.TrimSpace(res.Stdout()), "\n")
		// Check three containers are running
		assert.Assert(t, is.Len(out, 4))

		for _, cName := range []string{serverContainer, wordsContainer} {
			res = c.RunDockerCmd("inspect", cName)
			res.Assert(t, icmd.Success)

			containerInspect, err := ParseContainerInspect(res.Stdout())
			assert.NilError(t, err)
			assert.Assert(t, is.Len(containerInspect.Ports, 1))
			endpoint := fmt.Sprintf("http://%s:%d", containerInspect.Ports[0].HostIP, containerInspect.Ports[0].HostPort)
			t.Logf("Endpoint: %s", endpoint)
			var route string
			switch cName {
			case serverContainer:
				route = "/words/noun"
				assert.Equal(t, containerInspect.Ports[0].HostPort, uint32(80))
				assert.Equal(t, containerInspect.Ports[0].ContainerPort, uint32(80))
			case wordsContainer:
				route = "/noun"
				assert.Equal(t, containerInspect.Ports[0].HostPort, uint32(8080))
				assert.Equal(t, containerInspect.Ports[0].ContainerPort, uint32(8080))
			}
			checkUp := func(t poll.LogT) poll.Result {
				r, _ := http.Get(endpoint + route)
				if r != nil && r.StatusCode == http.StatusOK {
					return poll.Success()
				}
				return poll.Continue("Waiting for container to serve request")
			}
			poll.WaitOn(t, checkUp, poll.WithDelay(1*time.Second), poll.WithTimeout(60*time.Second))

			res = c.RunDockerCmd("ps")
			p := containerInspect.Ports[0]
			res.Assert(t, icmd.Expected{
				Out: fmt.Sprintf("%s:%d->%d/tcp", p.HostIP, p.HostPort, p.ContainerPort),
			})
		}
	})

	t.Run("down", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "down", "--project-name", composeProjectName)
		res.Assert(t, icmd.Success)

		res = c.RunDockerCmd("ps")
		res.Assert(t, icmd.Success)
		out := strings.Split(strings.TrimSpace(res.Stdout()), "\n")
		assert.Equal(t, len(out), 1)
	})
}

func TestRunEnvVars(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	_, _ = setupTestResourceGroup(t, c, "runEnv")

	t.Run("run", func(t *testing.T) {
		cmd := c.NewDockerCmd(
			"run", "-d",
			"-e", "MYSQL_ROOT_PASSWORD=rootpwd",
			"-e", "MYSQL_DATABASE=mytestdb",
			"-e", "MYSQL_USER",
			"-e", "MYSQL_PASSWORD=userpwd",
			"mysql:5.7",
		)
		cmd.Env = append(cmd.Env, "MYSQL_USER=user1")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Success)
		out := strings.Split(strings.TrimSpace(res.Stdout()), "\n")
		container := strings.TrimSpace(out[len(out)-1])
		t.Logf("Container name: %q", container)

		res = c.RunDockerCmd("inspect", container)
		res.Assert(t, icmd.Success)

		containerInspect, err := ParseContainerInspect(res.Stdout())
		assert.NilError(t, err)
		assert.Equal(t, containerInspect.Image, "mysql:5.7")

		check := func(t poll.LogT) poll.Result {
			res := c.RunDockerCmd("logs", container)
			if strings.Contains(res.Stdout(), "Giving user user1 access to schema mytestdb") {
				return poll.Success()
			}
			return poll.Continue("waiting for DB container to be up")
		}
		poll.WaitOn(t, check, poll.WithDelay(5*time.Second), poll.WithTimeout(60*time.Second))
	})
}

func setupTestResourceGroup(t *testing.T, c *E2eCLI, tName string) (string, string) {
	startTime := strconv.Itoa(int(time.Now().Unix()))
	rg := "E2E-" + tName + "-" + startTime
	azureLogin(t, c)
	sID := getSubscriptionID(t)
	t.Logf("Create resource group %q", rg)
	err := createResourceGroup(sID, rg)
	assert.Check(t, is.Nil(err))
	t.Cleanup(func() {
		if err := deleteResourceGroup(rg); err != nil {
			t.Error(err)
		}
	})
	createAciContextAndUseIt(t, c, sID, rg)
	// Check nothing is running
	res := c.RunDockerCmd("ps")
	res.Assert(t, icmd.Success)
	assert.Assert(t, is.Len(strings.Split(strings.TrimSpace(res.Stdout()), "\n"), 1))
	return sID, rg
}

func deleteResourceGroup(rgName string) error {
	ctx := context.TODO()
	helper := aci.NewACIResourceGroupHelper()
	models, err := helper.GetSubscriptionIDs(ctx)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return errors.New("unable to delete resource group: no models")
	}
	return helper.DeleteAsync(ctx, *models[0].SubscriptionID, rgName)
}

func azureLogin(t *testing.T, c *E2eCLI) {
	t.Log("Log in to Azure")
	// in order to create new service principal and get these 3 values : `az ad sp create-for-rbac --name 'TestServicePrincipal' --sdk-auth`
	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	tenantID := os.Getenv("AZURE_TENANT_ID")
	res := c.RunDockerCmd("login", "azure", "--client-id", clientID, "--client-secret", clientSecret, "--tenant-id", tenantID)
	res.Assert(t, icmd.Success)
}

func getSubscriptionID(t *testing.T) string {
	ctx := context.TODO()
	helper := aci.NewACIResourceGroupHelper()
	models, err := helper.GetSubscriptionIDs(ctx)
	assert.Check(t, is.Nil(err))
	assert.Check(t, len(models) == 1)
	return *models[0].SubscriptionID
}

func createResourceGroup(sID, rgName string) error {
	helper := aci.NewACIResourceGroupHelper()
	_, err := helper.CreateOrUpdate(context.TODO(), sID, rgName, resources.Group{Location: to.StringPtr(location)})
	return err
}

func createAciContextAndUseIt(t *testing.T, c *E2eCLI, sID, rgName string) {
	t.Log("Create ACI context")
	res := c.RunDockerCmd("context", "create", "aci", contextName, "--subscription-id", sID, "--resource-group", rgName, "--location", location)
	res.Assert(t, icmd.Success)
	res = c.RunDockerCmd("context", "use", contextName)
	res.Assert(t, icmd.Expected{Out: contextName})
	res = c.RunDockerCmd("context", "ls")
	res.Assert(t, icmd.Expected{Out: contextName + " *"})
}

func createStorageAccount(t *testing.T, aciContext store.AciContext, name string) (azure_storage.Account, func() error) {
	t.Logf("Create storage account %q", name)
	account, err := storage.CreateStorageAccount(context.TODO(), aciContext, name)
	assert.Check(t, is.Nil(err))
	assert.Check(t, is.Equal(*(account.Name), name))
	return account, func() error { return deleteStorageAccount(aciContext, name) }
}

func deleteStorageAccount(aciContext store.AciContext, name string) error {
	_, err := storage.DeleteStorageAccount(context.TODO(), aciContext, name)
	return err
}

func getStorageKeys(t *testing.T, aciContext store.AciContext, saName string) []azure_storage.AccountKey {
	l, err := storage.ListKeys(context.TODO(), aciContext, saName)
	assert.NilError(t, err)
	assert.Assert(t, l.Keys != nil)
	return *l.Keys
}

func createFileShare(t *testing.T, key, share, storageAccount string) (*azfile.SharedKeyCredential, *url.URL) {
	// Create a ShareURL object that wraps a soon-to-be-created share's URL and a default pipeline.
	u, _ := url.Parse(fmt.Sprintf("https://%s.file.core.windows.net/%s", storageAccount, share))
	cred, err := azfile.NewSharedKeyCredential(storageAccount, key)
	assert.NilError(t, err)

	shareURL := azfile.NewShareURL(*u, azfile.NewPipeline(cred, azfile.PipelineOptions{}))
	_, err = shareURL.Create(context.TODO(), azfile.Metadata{}, 0)
	assert.NilError(t, err)
	return cred, u
}

func uploadFile(t *testing.T, cred azfile.SharedKeyCredential, baseURL, fileName, content string) {
	fURL, err := url.Parse(baseURL + "/" + fileName)
	assert.NilError(t, err)
	fileURL := azfile.NewFileURL(*fURL, azfile.NewPipeline(&cred, azfile.PipelineOptions{}))
	err = azfile.UploadBufferToAzureFile(context.TODO(), []byte(content), fileURL, azfile.UploadToAzureFileOptions{})
	assert.NilError(t, err)
}

func getContainerName(stdout string) string {
	out := strings.Split(strings.TrimSpace(stdout), "\n")
	return strings.TrimSpace(out[len(out)-1])
}
