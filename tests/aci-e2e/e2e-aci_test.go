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

package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/docker/docker/pkg/fileutils"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/resources/mgmt/resources"
	"github.com/Azure/azure-storage-file-go/azfile"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/prometheus/tsdb/fileutil"

	"github.com/docker/compose-cli/aci"
	"github.com/docker/compose-cli/aci/convert"
	"github.com/docker/compose-cli/aci/login"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/errdefs"
	. "github.com/docker/compose-cli/tests/framework"
)

const (
	contextName = "aci-test"
)

var (
	binDir   string
	location = []string{"eastus2"}
)

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
		location := getTestLocation()
		err := createResourceGroup(t, sID, rg, location)
		assert.Check(t, is.Nil(err))
		t.Cleanup(func() {
			_ = deleteResourceGroup(t, rg)
		})

		c.RunDockerCmd("context", "create", "aci", contextName, "--subscription-id", sID, "--resource-group", rg, "--location", location)
		res := c.RunDockerCmd("context", "use", contextName)
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
		res := c.RunDockerOrExitError("context", "create", "aci", "fail-context")
		res.Assert(t, icmd.Expected{
			ExitCode: errdefs.ExitCodeLoginRequired,
			Err:      `not logged in to azure, you need to run "docker login azure" first`,
		})
	})
}

func getTestLocation() string {
	rand.Seed(time.Now().Unix())
	n := rand.Intn(len(location))
	return location[n]
}

func uploadTestFile(t *testing.T, aciContext store.AciContext, accountName string, fileshareName string, testFileName string, testFileContent string) {
	storageLogin := login.StorageLoginImpl{AciContext: aciContext}
	key, err := storageLogin.GetAzureStorageAccountKey(context.TODO(), accountName)
	assert.NilError(t, err)
	cred, err := azfile.NewSharedKeyCredential(accountName, key)
	assert.NilError(t, err)
	u, _ := url.Parse(fmt.Sprintf("https://%s.file.core.windows.net/%s", accountName, fileshareName))
	uploadFile(t, *cred, u.String(), testFileName, testFileContent)
}

const (
	fileshareName  = "dockertestshare"
	fileshareName2 = "dockertestshare2"
)

func TestRunVolume(t *testing.T) {
	const (
		testFileContent = "Volume mounted successfully!"
		testFileName    = "index.html"
	)

	c := NewParallelE2eCLI(t, binDir)
	sID, rg, location := setupTestResourceGroup(t, c)

	// Bootstrap volume
	aciContext := store.AciContext{
		SubscriptionID: sID,
		Location:       location,
		ResourceGroup:  rg,
	}

	// Used in subtests
	var (
		container   string
		hostIP      string
		endpoint    string
		volumeID    string
		accountName = "e2e" + strconv.Itoa(int(time.Now().UnixNano()))
	)

	t.Run("check empty volume name validity", func(t *testing.T) {
		invalidName := ""
		res := c.RunDockerOrExitError("volume", "create", "--storage-account", invalidName, fileshareName)
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      `parameter=accountName constraint=MinLength value="" details: value length must be greater than or equal to 3`,
		})
	})

	t.Run("check volume name validity", func(t *testing.T) {
		invalidName := "some-storage-123"
		res := c.RunDockerOrExitError("volume", "create", "--storage-account", invalidName, fileshareName)
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "some-storage-123 is not a valid storage account name. Storage account name must be between 3 and 24 characters in length and use numbers and lower-case letters only.",
		})
	})

	t.Run("create volumes", func(t *testing.T) {
		c.RunDockerCmd("volume", "create", "--storage-account", accountName, fileshareName)
	})

	volumeID = accountName + "/" + fileshareName
	t.Cleanup(func() {
		c.RunDockerCmd("volume", "rm", volumeID)
		res := c.RunDockerCmd("volume", "ls")
		lines := lines(res.Stdout())
		assert.Equal(t, len(lines), 1)
	})

	t.Run("create second fileshare", func(t *testing.T) {
		c.RunDockerCmd("volume", "create", "--storage-account", accountName, fileshareName2)
	})
	volumeID2 := accountName + "/" + fileshareName2

	t.Run("list volumes", func(t *testing.T) {
		res := c.RunDockerCmd("volume", "ls", "--quiet")
		l := lines(res.Stdout())
		assert.Equal(t, len(l), 2)
		assert.Equal(t, l[0], volumeID)
		assert.Equal(t, l[1], volumeID2)

		res = c.RunDockerCmd("volume", "ls")
		l = lines(res.Stdout())
		assert.Equal(t, len(l), 3)
		firstAccount := l[1]
		fields := strings.Fields(firstAccount)
		assert.Equal(t, fields[0], volumeID)
		secondAccount := l[2]
		fields = strings.Fields(secondAccount)
		assert.Equal(t, fields[0], volumeID2)
	})

	t.Run("inspect volumes", func(t *testing.T) {
		res := c.RunDockerCmd("volume", "inspect", volumeID)
		assert.Equal(t, res.Stdout(), fmt.Sprintf(`{
    "ID": %q,
    "Description": "Fileshare %s in %s storage account"
}
`, volumeID, fileshareName, accountName))
		res = c.RunDockerCmd("volume", "inspect", volumeID2)
		assert.Equal(t, res.Stdout(), fmt.Sprintf(`{
    "ID": %q,
    "Description": "Fileshare %s in %s storage account"
}
`, volumeID2, fileshareName2, accountName))
	})

	t.Run("delete only fileshare", func(t *testing.T) {
		c.RunDockerCmd("volume", "rm", volumeID2)
		res := c.RunDockerCmd("volume", "ls")
		lines := lines(res.Stdout())
		assert.Equal(t, len(lines), 2)
		assert.Assert(t, !strings.Contains(res.Stdout(), fileshareName2), "second fileshare still visible after rm")
	})

	t.Run("upload file", func(t *testing.T) {
		uploadTestFile(t, aciContext, accountName, fileshareName, testFileName, testFileContent)
	})

	t.Run("run", func(t *testing.T) {
		mountTarget := "/usr/share/nginx/html"
		res := c.RunDockerCmd(
			"run", "-d",
			"-v", fmt.Sprintf("%s:%s", volumeID, mountTarget),
			"-p", "80:80",
			"nginx",
		)
		container = getContainerName(res.Stdout())
	})

	t.Run("inspect", func(t *testing.T) {
		res := c.RunDockerCmd("inspect", container)

		containerInspect, err := ParseContainerInspect(res.Stdout())
		assert.NilError(t, err)
		assert.Equal(t, containerInspect.Platform, "Linux")
		assert.Equal(t, containerInspect.HostConfig.CPULimit, 1.0)
		assert.Equal(t, containerInspect.HostConfig.CPUReservation, 1.0)
		assert.Equal(t, containerInspect.HostConfig.RestartPolicy, containers.RestartPolicyNone)

		assert.Assert(t, is.Len(containerInspect.Ports, 1))
		hostIP = containerInspect.Ports[0].HostIP
		endpoint = fmt.Sprintf("http://%s:%d", containerInspect.Ports[0].HostIP, containerInspect.Ports[0].HostPort)
	})

	t.Run("ps", func(t *testing.T) {
		res := c.RunDockerCmd("ps")
		out := lines(res.Stdout())
		l := out[len(out)-1]
		assert.Assert(t, strings.Contains(l, container), "Looking for %q in line: %s", container, l)
		assert.Assert(t, strings.Contains(l, "nginx"))
		assert.Assert(t, strings.Contains(l, "Running"))
		assert.Assert(t, strings.Contains(l, hostIP+":80->80/tcp"))
	})

	t.Run("http get", func(t *testing.T) {
		output := HTTPGetWithRetry(t, endpoint, http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, testFileContent), "Actual content: "+output)
	})

	t.Run("logs", func(t *testing.T) {
		res := c.RunDockerCmd("logs", container)
		res.Assert(t, icmd.Expected{Out: "GET"})
	})

	t.Run("exec", func(t *testing.T) {
		res := c.RunDockerOrExitError("exec", container, "pwd")
		assert.Assert(t, strings.Contains(res.Stdout(), "/"))

		res = c.RunDockerOrExitError("exec", container, "echo", "fail_with_argument")
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
		res := c.RunDockerOrExitError("rm", container)
		res.Assert(t, icmd.Expected{
			Err:      fmt.Sprintf("Error: you cannot remove a running container %s. Stop the container before attempting removal or force remove", container),
			ExitCode: 1,
		})
	})

	t.Run("force rm", func(t *testing.T) {
		res := c.RunDockerCmd("rm", "-f", container)
		res.Assert(t, icmd.Expected{Out: container})

		checkStopped := func(t poll.LogT) poll.Result {
			res := c.RunDockerOrExitError("inspect", container)
			if res.ExitCode == 1 {
				return poll.Success()
			}
			return poll.Continue("waiting for container to stop")
		}
		poll.WaitOn(t, checkStopped, poll.WithDelay(5*time.Second), poll.WithTimeout(60*time.Second))
	})
}

func lines(output string) []string {
	return strings.Split(strings.TrimSpace(output), "\n")
}

func TestContainerRunAttached(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	_, groupID, location := setupTestResourceGroup(t, c)

	// Used in subtests
	var (
		container         string = "test-container"
		endpoint          string
		followLogsProcess *icmd.Result
	)

	t.Run("run attached limits", func(t *testing.T) {
		dnsLabelName := "nginx-" + groupID
		fqdn := dnsLabelName + "." + location + ".azurecontainer.io"

		cmd := c.NewDockerCmd(
			"run",
			"--name", container,
			"--restart", "on-failure",
			"--memory", "0.1G", "--cpus", "0.1",
			"-p", "80:80",
			"--domainname",
			dnsLabelName,
			"nginx",
		)
		followLogsProcess = icmd.StartCmd(cmd)

		checkRunning := func(t poll.LogT) poll.Result {
			res := c.RunDockerOrExitError("inspect", container)
			if res.ExitCode == 0 && strings.Contains(res.Stdout(), `"Status": "Running"`) && !strings.Contains(res.Stdout(), `"HostIP": ""`) {
				return poll.Success()
			}
			return poll.Continue("waiting for container to be running, current inspect result: \n%s", res.Combined())
		}
		poll.WaitOn(t, checkRunning, poll.WithDelay(5*time.Second), poll.WithTimeout(90*time.Second))

		inspectRes := c.RunDockerCmd("inspect", container)

		containerInspect, err := ParseContainerInspect(inspectRes.Stdout())
		assert.NilError(t, err)
		assert.Equal(t, containerInspect.Platform, "Linux")
		assert.Equal(t, containerInspect.HostConfig.CPULimit, 0.1)
		assert.Equal(t, containerInspect.HostConfig.MemoryLimit, uint64(107374182))
		assert.Equal(t, containerInspect.HostConfig.CPUReservation, 0.1)
		assert.Equal(t, containerInspect.HostConfig.MemoryReservation, uint64(107374182))
		assert.Equal(t, containerInspect.HostConfig.RestartPolicy, containers.RestartPolicyOnFailure)

		assert.Assert(t, is.Len(containerInspect.Ports, 1))
		port := containerInspect.Ports[0]
		assert.Assert(t, port.HostIP != "", "empty hostIP, inspect: \n"+inspectRes.Stdout())
		assert.Equal(t, port.ContainerPort, uint32(80))
		assert.Equal(t, port.HostPort, uint32(80))
		assert.Equal(t, containerInspect.Config.FQDN, fqdn)
		endpoint = fmt.Sprintf("http://%s:%d", fqdn, port.HostPort)

		assert.Assert(t, !strings.Contains(followLogsProcess.Stdout(), "/test"))
		HTTPGetWithRetry(t, endpoint+"/test", http.StatusNotFound, 2*time.Second, 60*time.Second)

		checkLog := func(t poll.LogT) poll.Result {
			if strings.Contains(followLogsProcess.Stdout(), "/test") {
				return poll.Success()
			}
			return poll.Continue("waiting for logs to contain /test")
		}
		poll.WaitOn(t, checkLog, poll.WithDelay(1*time.Second), poll.WithTimeout(20*time.Second))
	})

	t.Run("stop wrong container", func(t *testing.T) {
		res := c.RunDockerOrExitError("stop", "unknown-container")
		res.Assert(t, icmd.Expected{
			Err:      "Error: container unknown-container not found",
			ExitCode: 1,
		})
	})

	t.Run("stop container", func(t *testing.T) {
		res := c.RunDockerCmd("stop", container)
		res.Assert(t, icmd.Expected{Out: container})
		waitForStatus(t, c, container, "Terminated", "Node Stopped")
	})

	t.Run("check we stoppped following logs", func(t *testing.T) {
		// nolint errcheck
		followLogsStopped := waitWithTimeout(func() { followLogsProcess.Cmd.Process.Wait() }, 10*time.Second)
		assert.NilError(t, followLogsStopped, "Follow logs process did not stop after container is stopped")
	})

	t.Run("ps stopped container with --all", func(t *testing.T) {
		res := c.RunDockerCmd("ps", container)
		out := lines(res.Stdout())
		assert.Assert(t, is.Len(out, 1))

		res = c.RunDockerCmd("ps", "--all", container)
		out = lines(res.Stdout())
		assert.Assert(t, is.Len(out, 2))
	})

	t.Run("restart container", func(t *testing.T) {
		res := c.RunDockerCmd("start", container)
		res.Assert(t, icmd.Expected{Out: container})
		waitForStatus(t, c, container, convert.StatusRunning)
	})

	t.Run("prune dry run", func(t *testing.T) {
		res := c.RunDockerCmd("prune", "--dry-run")
		assert.Equal(t, "Resources that would be deleted:\nTotal CPUs reclaimed: 0.00, total memory reclaimed: 0.00 GB\n", res.Stdout())
		res = c.RunDockerCmd("prune", "--dry-run", "--force")
		assert.Equal(t, "Resources that would be deleted:\n"+container+"\nTotal CPUs reclaimed: 0.10, total memory reclaimed: 0.10 GB\n", res.Stdout())
	})

	t.Run("prune", func(t *testing.T) {
		res := c.RunDockerCmd("prune")
		assert.Equal(t, "Deleted resources:\nTotal CPUs reclaimed: 0.00, total memory reclaimed: 0.00 GB\n", res.Stdout())
		res = c.RunDockerCmd("ps")
		l := lines(res.Stdout())
		assert.Equal(t, 2, len(l))

		res = c.RunDockerCmd("prune", "--force")
		assert.Equal(t, "Deleted resources:\n"+container+"\nTotal CPUs reclaimed: 0.10, total memory reclaimed: 0.10 GB\n", res.Stdout())

		res = c.RunDockerCmd("ps", "--all")
		l = lines(res.Stdout())
		assert.Equal(t, 1, len(l))
	})
}

func overwriteFileStorageAccount(t *testing.T, absComposefileName string, storageAccount string) {
	data, err := ioutil.ReadFile(absComposefileName)
	assert.NilError(t, err)
	override := strings.Replace(string(data), "dockertestvolumeaccount", storageAccount, 1)
	err = ioutil.WriteFile(absComposefileName, []byte(override), 0644)
	assert.NilError(t, err)
}

func TestUpSecretsResources(t *testing.T) {
	const (
		composeProjectName = "aci_test"
		web1               = composeProjectName + "_web1"
		web2               = composeProjectName + "_web2"

		secret1Name  = "mytarget1"
		secret1Value = "myPassword1\n"

		secret2Name  = "mysecret2"
		secret2Value = "another_password\n"
	)
	var (
		basefilePath    = filepath.Join("..", "composefiles", "aci_secrets_resources")
		composefilePath = filepath.Join(basefilePath, "compose.yml")
	)
	c := NewParallelE2eCLI(t, binDir)
	_, _, _ = setupTestResourceGroup(t, c)

	t.Run("compose up", func(t *testing.T) {
		c.RunDockerCmd("compose", "up", "-f", composefilePath, "--project-name", composeProjectName)
		res := c.RunDockerCmd("ps")
		out := lines(res.Stdout())
		// Check 2 containers running
		assert.Assert(t, is.Len(out, 3))
	})

	t.Cleanup(func() {
		c.RunDockerCmd("compose", "down", "--project-name", composeProjectName)
		res := c.RunDockerCmd("ps")
		out := lines(res.Stdout())
		assert.Equal(t, len(out), 1)
	})

	res := c.RunDockerCmd("inspect", web1)
	web1Inspect, err := ParseContainerInspect(res.Stdout())
	assert.NilError(t, err)
	res = c.RunDockerCmd("inspect", web2)
	web2Inspect, err := ParseContainerInspect(res.Stdout())
	assert.NilError(t, err)

	t.Run("read secrets in service 1", func(t *testing.T) {
		assert.Assert(t, is.Len(web1Inspect.Ports, 1))
		endpoint := fmt.Sprintf("http://%s:%d", web1Inspect.Ports[0].HostIP, web1Inspect.Ports[0].HostPort)

		output := HTTPGetWithRetry(t, endpoint+"/"+secret1Name, http.StatusOK, 2*time.Second, 20*time.Second)
		// replace windows carriage return
		output = strings.ReplaceAll(output, "\r", "")
		assert.Equal(t, output, secret1Value)

		output = HTTPGetWithRetry(t, endpoint+"/"+secret2Name, http.StatusOK, 2*time.Second, 20*time.Second)
		output = strings.ReplaceAll(output, "\r", "")
		assert.Equal(t, output, secret2Value)
	})

	t.Run("read secrets in service 2", func(t *testing.T) {
		assert.Assert(t, is.Len(web2Inspect.Ports, 1))
		endpoint := fmt.Sprintf("http://%s:%d", web2Inspect.Ports[0].HostIP, web2Inspect.Ports[0].HostPort)

		output := HTTPGetWithRetry(t, endpoint+"/"+secret2Name, http.StatusOK, 2*time.Second, 20*time.Second)
		output = strings.ReplaceAll(output, "\r", "")
		assert.Equal(t, output, secret2Value)

		HTTPGetWithRetry(t, endpoint+"/"+secret1Name, http.StatusNotFound, 2*time.Second, 20*time.Second)
	})

	t.Run("check resource limits", func(t *testing.T) {
		assert.Equal(t, web1Inspect.HostConfig.CPULimit, 0.7)
		assert.Equal(t, web1Inspect.HostConfig.MemoryLimit, uint64(1073741824))
		assert.Equal(t, web1Inspect.HostConfig.CPUReservation, 0.5)
		assert.Equal(t, web1Inspect.HostConfig.MemoryReservation, uint64(536870912))

		assert.Equal(t, web2Inspect.HostConfig.CPULimit, 0.5)
		assert.Equal(t, web2Inspect.HostConfig.MemoryLimit, uint64(751619276))
		assert.Equal(t, web2Inspect.HostConfig.CPUReservation, 0.5)
		assert.Equal(t, web2Inspect.HostConfig.MemoryReservation, uint64(751619276))
	})

	t.Run("check healthchecks inspect", func(t *testing.T) {
		assert.Equal(t, web1Inspect.Healthcheck.Disable, false)
		assert.Equal(t, time.Duration(web1Inspect.Healthcheck.Interval), 5*time.Second)
		assert.DeepEqual(t, web1Inspect.Healthcheck.Test, []string{"curl", "-f", "http://localhost:80/healthz"})

		assert.Equal(t, web2Inspect.Healthcheck.Disable, true)
	})

	t.Run("healthcheck restart failed app", func(t *testing.T) {
		endpoint := fmt.Sprintf("http://%s:%d", web1Inspect.Ports[0].HostIP, web1Inspect.Ports[0].HostPort)
		HTTPGetWithRetry(t, endpoint+"/failtestserver", http.StatusOK, 3*time.Second, 3*time.Second)

		logs := c.RunDockerCmd("logs", web1).Combined()
		assert.Assert(t, strings.Contains(logs, "GET /healthz"))
		assert.Assert(t, strings.Contains(logs, "GET /failtestserver"))
		assert.Assert(t, strings.Contains(logs, "Server failing"))

		checkLogsReset := func(logt poll.LogT) poll.Result {
			res := c.RunDockerOrExitError("logs", web1)
			if res.ExitCode == 0 &&
				!strings.Contains(res.Combined(), "GET /failtestserver") &&
				strings.Contains(res.Combined(), "Listening on port 80") &&
				strings.Contains(res.Combined(), "GET /healthz") {
				return poll.Success()
			}
			return poll.Continue("Logs not reset by healcheck restart\n" + res.Combined())
		}

		poll.WaitOn(t, checkLogsReset, poll.WithDelay(5*time.Second), poll.WithTimeout(90*time.Second))

		res := c.RunDockerCmd("inspect", web1)
		web1Inspect, err = ParseContainerInspect(res.Stdout())
		assert.Equal(t, web1Inspect.Status, "Running")
	})
}

func TestUpUpdate(t *testing.T) {
	const (
		composeProjectName = "acidemo"
		serverContainer    = composeProjectName + "_web"
		wordsContainer     = composeProjectName + "_words"
		dbContainer        = composeProjectName + "_db"
	)
	var (
		singlePortVolumesComposefile = "aci_demo_port_volumes.yaml"
		multiPortComposefile         = "demo_multi_port.yaml"
	)
	c := NewParallelE2eCLI(t, binDir)
	sID, groupID, location := setupTestResourceGroup(t, c)
	composeAccountName := groupID + "-sa"
	composeAccountName = strings.ReplaceAll(composeAccountName, "-", "")
	composeAccountName = strings.ToLower(composeAccountName)

	dstDir := filepath.Join(os.TempDir(), "e2e-aci-volume-"+composeAccountName)
	srcDir := filepath.Join("..", "composefiles", "aci-demo")
	err := fileutil.CopyDirs(srcDir, dstDir)
	assert.NilError(t, err)
	t.Cleanup(func() {
		assert.NilError(t, os.RemoveAll(dstDir))
	})
	_, err = fileutils.CopyFile(filepath.Join(filepath.Join("..", "composefiles"), multiPortComposefile), filepath.Join(dstDir, multiPortComposefile))
	assert.NilError(t, err)

	singlePortVolumesComposefile = filepath.Join(dstDir, singlePortVolumesComposefile)
	overwriteFileStorageAccount(t, singlePortVolumesComposefile, composeAccountName)
	multiPortComposefile = filepath.Join(dstDir, multiPortComposefile)

	volumeID := composeAccountName + "/" + fileshareName
	t.Run("compose up", func(t *testing.T) {
		const (
			testFileName    = "msg.txt"
			testFileContent = "VOLUME_OK"
			projectName     = "acidemo"
		)

		c.RunDockerCmd("volume", "create", "--storage-account", composeAccountName, fileshareName)

		// Bootstrap volume
		aciContext := store.AciContext{
			SubscriptionID: sID,
			Location:       location,
			ResourceGroup:  groupID,
		}
		uploadTestFile(t, aciContext, composeAccountName, fileshareName, testFileName, testFileContent)

		dnsLabelName := "nginx-" + groupID
		fqdn := dnsLabelName + "." + location + ".azurecontainer.io"
		// Name of Compose project is taken from current folder "acie2e"
		c.RunDockerCmd("compose", "up", "-f", singlePortVolumesComposefile, "--domainname", dnsLabelName, "--project-name", projectName)

		res := c.RunDockerCmd("ps")
		out := lines(res.Stdout())
		// Check three containers are running
		assert.Assert(t, is.Len(out, 4))
		webRunning := false
		for _, l := range out {
			if strings.Contains(l, serverContainer) {
				webRunning = true
				strings.Contains(l, ":80->80/tcp")
			}
		}
		assert.Assert(t, webRunning, "web container not running ; ps:\n"+res.Stdout())

		res = c.RunDockerCmd("inspect", serverContainer)

		containerInspect, err := ParseContainerInspect(res.Stdout())
		assert.NilError(t, err)
		assert.Assert(t, is.Len(containerInspect.Ports, 1))
		endpoint := fmt.Sprintf("http://%s:%d", containerInspect.Ports[0].HostIP, containerInspect.Ports[0].HostPort)

		output := HTTPGetWithRetry(t, endpoint+"/words/noun", http.StatusOK, 2*time.Second, 20*time.Second)

		assert.Assert(t, strings.Contains(output, `"word":`))

		endpoint = fmt.Sprintf("http://%s:%d", fqdn, containerInspect.Ports[0].HostPort)
		HTTPGetWithRetry(t, endpoint+"/words/noun", http.StatusOK, 2*time.Second, 20*time.Second)

		body := HTTPGetWithRetry(t, endpoint+"/volume_test/"+testFileName, http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(body, testFileContent))

		// Try to remove the volume while it's still in use
		res = c.RunDockerOrExitError("volume", "rm", volumeID)
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err: fmt.Sprintf(`Error: volume "%s/%s" is used in container group %q`,
				composeAccountName, fileshareName, projectName),
		})
	})
	t.Cleanup(func() {
		c.RunDockerCmd("volume", "rm", volumeID)
	})

	t.Run("compose ps", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "ps", "--project-name", composeProjectName, "--quiet")
		l := lines(res.Stdout())
		assert.Assert(t, is.Len(l, 3))

		res = c.RunDockerCmd("compose", "ps", "--project-name", composeProjectName)
		l = lines(res.Stdout())
		assert.Assert(t, is.Len(l, 4))
		var wordsDisplayed, webDisplayed, dbDisplayed bool
		for _, line := range l {
			fields := strings.Fields(line)
			containerID := fields[0]
			switch containerID {
			case wordsContainer:
				wordsDisplayed = true
				assert.DeepEqual(t, fields, []string{containerID, "words", "1/1"})
			case dbContainer:
				dbDisplayed = true
				assert.DeepEqual(t, fields, []string{containerID, "db", "1/1"})
			case serverContainer:
				webDisplayed = true
				assert.Equal(t, fields[1], "web")
				assert.Check(t, strings.Contains(fields[3], ":80->80/tcp"))
			}
		}
		assert.Check(t, webDisplayed && wordsDisplayed && dbDisplayed, "\n%s\n", res.Stdout())
	})

	t.Run("compose ls", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "ls", "--quiet")
		l := lines(res.Stdout())
		assert.Assert(t, is.Len(l, 1))
		res = c.RunDockerCmd("compose", "ls")
		l = lines(res.Stdout())

		assert.Equal(t, 2, len(l))
		fields := strings.Fields(l[1])
		assert.Equal(t, 2, len(fields))
		assert.Equal(t, fields[0], composeProjectName)
		assert.Equal(t, "Running", fields[1])
	})

	t.Run("logs web", func(t *testing.T) {
		res := c.RunDockerCmd("logs", serverContainer)
		res.Assert(t, icmd.Expected{Out: "Listening on port 80"})
	})

	t.Run("update", func(t *testing.T) {
		c.RunDockerCmd("compose", "up", "-f", multiPortComposefile, "--project-name", composeProjectName)
		res := c.RunDockerCmd("ps")
		out := lines(res.Stdout())
		// Check three containers are running
		assert.Assert(t, is.Len(out, 4))

		for _, cName := range []string{serverContainer, wordsContainer} {
			res = c.RunDockerCmd("inspect", cName)

			containerInspect, err := ParseContainerInspect(res.Stdout())
			assert.NilError(t, err)
			assert.Assert(t, is.Len(containerInspect.Ports, 1))
			endpoint := fmt.Sprintf("http://%s:%d", containerInspect.Ports[0].HostIP, containerInspect.Ports[0].HostPort)
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
			HTTPGetWithRetry(t, endpoint+route, http.StatusOK, 1*time.Second, 60*time.Second)

			res = c.RunDockerCmd("ps")
			p := containerInspect.Ports[0]
			res.Assert(t, icmd.Expected{
				Out: fmt.Sprintf("%s:%d->%d/tcp", p.HostIP, p.HostPort, p.ContainerPort),
			})
		}
	})

	t.Run("down", func(t *testing.T) {
		c.RunDockerCmd("compose", "down", "--project-name", composeProjectName)
		res := c.RunDockerCmd("ps")
		out := lines(res.Stdout())
		assert.Equal(t, len(out), 1)
	})
}

/*
func TestRunEnvVars(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	_, _, _ = setupTestResourceGroup(t, c)

	t.Run("run", func(t *testing.T) {
		cmd := c.NewDockerCmd(
			"run", "-d",
			"-e", "MYSQL_ROOT_PASSWORD=rootpwd",
			"-e", "MYSQL_DATABASE=mytestdb",
			"-e", "MYSQL_USER",
			"-e", "MYSQL_PASSWORD=userpwd",
			"-e", "DATASOURCE_URL=jdbc:mysql://mydb.mysql.database.azure.com/db1?useSSL=true&requireSSL=false&serverTimezone=America/Recife",
			"mysql:5.7",
		)
		cmd.Env = append(cmd.Env, "MYSQL_USER=user1")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Success)
		out := lines(res.Stdout())
		container := strings.TrimSpace(out[len(out)-1])

		res = c.RunDockerCmd("inspect", container)

		containerInspect, err := ParseContainerInspect(res.Stdout())
		assert.NilError(t, err)
		assert.Assert(t, containerInspect.Config != nil, "nil container config")
		assert.Assert(t, containerInspect.Config.Env != nil, "nil container env variables")
		assert.Equal(t, containerInspect.Image, "mysql:5.7")
		envVars := containerInspect.Config.Env
		assert.Equal(t, len(envVars), 5)
		assert.Equal(t, envVars["MYSQL_ROOT_PASSWORD"], "rootpwd")
		assert.Equal(t, envVars["MYSQL_DATABASE"], "mytestdb")
		assert.Equal(t, envVars["MYSQL_USER"], "user1")
		assert.Equal(t, envVars["MYSQL_PASSWORD"], "userpwd")
		assert.Equal(t, envVars["DATASOURCE_URL"], "jdbc:mysql://mydb.mysql.database.azure.com/db1?useSSL=true&requireSSL=false&serverTimezone=America/Recife")

		check := func(t poll.LogT) poll.Result {
			res := c.RunDockerOrExitError("logs", container)
			if strings.Contains(res.Stdout(), "Giving user user1 access to schema mytestdb") {
				return poll.Success()
			}
			return poll.Continue("waiting for DB container to be up\n%s", res.Combined())
		}
		poll.WaitOn(t, check, poll.WithDelay(5*time.Second), poll.WithTimeout(90*time.Second))
	})
}
*/

func setupTestResourceGroup(t *testing.T, c *E2eCLI) (string, string, string) {
	startTime := strconv.Itoa(int(time.Now().Unix()))
	rg := "E2E-" + t.Name() + "-" + startTime[5:]
	azureLogin(t, c)
	sID := getSubscriptionID(t)
	location := getTestLocation()
	err := createResourceGroup(t, sID, rg, location)
	assert.Check(t, is.Nil(err))
	t.Cleanup(func() {
		if err := deleteResourceGroup(t, rg); err != nil {
			t.Error(err)
		}
	})

	createAciContextAndUseIt(t, c, sID, rg, location)
	// Check nothing is running
	res := c.RunDockerCmd("ps")
	assert.Assert(t, is.Len(lines(res.Stdout()), 1))
	return sID, rg, location
}

func deleteResourceGroup(t *testing.T, rgName string) error {
	fmt.Printf("	[%s] deleting resource group %s\n", t.Name(), rgName)
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
	// in order to create new service principal and get these 3 values : `az ad sp create-for-rbac --name 'TestServicePrincipal' --sdk-auth`
	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	tenantID := os.Getenv("AZURE_TENANT_ID")
	assert.Check(t, clientID != "", "AZURE_CLIENT_ID must not be empty")
	assert.Check(t, clientSecret != "", "AZURE_CLIENT_SECRET must not be empty")
	assert.Check(t, tenantID != "", "AZURE_TENANT_ID must not be empty")
	c.RunDockerCmd("login", "azure", "--client-id", clientID, "--client-secret", clientSecret, "--tenant-id", tenantID)
}

func getSubscriptionID(t *testing.T) string {
	ctx := context.TODO()
	helper := aci.NewACIResourceGroupHelper()
	models, err := helper.GetSubscriptionIDs(ctx)
	assert.Check(t, is.Nil(err))
	assert.Check(t, len(models) == 1)
	return *models[0].SubscriptionID
}

func createResourceGroup(t *testing.T, sID, rgName string, location string) error {
	fmt.Printf("	[%s] creating resource group %s\n", t.Name(), rgName)
	helper := aci.NewACIResourceGroupHelper()
	_, err := helper.CreateOrUpdate(context.TODO(), sID, rgName, resources.Group{Location: to.StringPtr(location)})
	return err
}

func createAciContextAndUseIt(t *testing.T, c *E2eCLI, sID, rgName string, location string) {
	res := c.RunDockerCmd("context", "create", "aci", contextName, "--subscription-id", sID, "--resource-group", rgName, "--location", location)
	res.Assert(t, icmd.Expected{Out: "Successfully created aci context \"" + contextName + "\""})
	res = c.RunDockerCmd("context", "use", contextName)
	res.Assert(t, icmd.Expected{Out: contextName})
	res = c.RunDockerCmd("context", "ls")
	res.Assert(t, icmd.Expected{Out: contextName + " *"})
}

func uploadFile(t *testing.T, cred azfile.SharedKeyCredential, baseURL, fileName, content string) {
	fURL, err := url.Parse(baseURL + "/" + fileName)
	assert.NilError(t, err)
	fileURL := azfile.NewFileURL(*fURL, azfile.NewPipeline(&cred, azfile.PipelineOptions{}))
	err = azfile.UploadBufferToAzureFile(context.TODO(), []byte(content), fileURL, azfile.UploadToAzureFileOptions{})
	assert.NilError(t, err)
}

func getContainerName(stdout string) string {
	out := lines(stdout)
	return strings.TrimSpace(out[len(out)-1])
}

func waitForStatus(t *testing.T, c *E2eCLI, containerID string, statuses ...string) {
	checkStopped := func(logt poll.LogT) poll.Result {
		res := c.RunDockerCmd("inspect", containerID)
		containerInspect, err := ParseContainerInspect(res.Stdout())
		assert.NilError(t, err)
		for _, status := range statuses {
			if containerInspect.Status == status {
				return poll.Success()
			}
		}
		return poll.Continue("Status %s != %s (expected) for container %s", containerInspect.Status, statuses, containerID)
	}

	poll.WaitOn(t, checkStopped, poll.WithDelay(5*time.Second), poll.WithTimeout(90*time.Second))
}

func waitWithTimeout(blockingCall func(), timeout time.Duration) error {
	c := make(chan struct{})
	go func() {
		defer close(c)
		blockingCall()
	}()
	select {
	case <-c:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("Timed out after %s", timeout)
	}
}
