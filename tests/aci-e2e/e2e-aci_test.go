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
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/resources/mgmt/resources"
	azure_storage "github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/storage/mgmt/storage"
	"github.com/Azure/azure-storage-file-go/azfile"
	"github.com/Azure/go-autorest/autorest/to"
	. "github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"

	"github.com/docker/api/azure"
	"github.com/docker/api/azure/login"
	"github.com/docker/api/context/store"
	"github.com/docker/api/tests/aci-e2e/storage"
	. "github.com/docker/api/tests/framework"
)

const (
	location          = "westeurope"
	contextName       = "acitest"
	testContainerName = "testcontainername"
	testShareName     = "dockertestshare"
	testFileContent   = "Volume mounted with success!"
	testFileName      = "index.html"
)

var (
	subscriptionID         string
	resourceGroupName      = "resourceGroupTestE2E-" + RandStringBytes(10)
	testStorageAccountName = "storageteste2e" + RandStringBytes(6) // "between 3 and 24 characters in length and use numbers and lower-case letters only"
)

type E2eACISuite struct {
	Suite
}

func (s *E2eACISuite) TestContextDefault() {
	s.T().Run("should be initialized with default context", func(t *testing.T) {
		_, err := s.NewCommand("docker", "context", "rm", "-f", contextName).Exec()
		if err == nil {
			log.Println("Cleaning existing test context")
		}

		s.NewCommand("docker", "context", "use", "default").ExecOrDie()
		output := s.NewCommand("docker", "context", "ls").ExecOrDie()
		Expect(output).To(Not(ContainSubstring(contextName)))
		Expect(output).To(ContainSubstring("default *"))
	})
}

func (s *E2eACISuite) TestACIBackend() {
	s.T().Run("Logs in azure using service principal credentials", func(t *testing.T) {
		login, err := login.NewAzureLoginService()
		Expect(err).To(BeNil())
		// in order to create new service principal and get these 3 values : `az ad sp create-for-rbac --name 'TestServicePrincipal' --sdk-auth`
		clientID := os.Getenv("AZURE_CLIENT_ID")
		clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
		tenantID := os.Getenv("AZURE_TENANT_ID")
		err = login.TestLoginFromServicePrincipal(clientID, clientSecret, tenantID)
		Expect(err).To(BeNil())
	})

	s.T().Run("creates a new aci context for tests", func(t *testing.T) {
		setupTestResourceGroup(resourceGroupName)
		helper := azure.NewACIResourceGroupHelper()
		models, err := helper.GetSubscriptionIDs(context.TODO())
		Expect(err).To(BeNil())
		subscriptionID = *models[0].SubscriptionID

		s.NewDockerCommand("context", "create", "aci", contextName, "--subscription-id", subscriptionID, "--resource-group", resourceGroupName, "--location", location).ExecOrDie()
	})

	defer deleteResourceGroup(resourceGroupName)

	s.T().Run("uses the aci context", func(t *testing.T) {
		currentContext := s.NewCommand("docker", "context", "use", contextName).ExecOrDie()
		Expect(currentContext).To(ContainSubstring(contextName))
		output := s.NewCommand("docker", "context", "ls").ExecOrDie()
		Expect(output).To(ContainSubstring("acitest *"))
	})

	It("ensures no container is running initially", func() {
		output := s.NewDockerCommand("ps").ExecOrDie()
		Expect(len(Lines(output))).To(Equal(1))
	})

	var nginxExposedURL string
	s.T().Run("runs nginx on port 80", func(t *testing.T) {
		aciContext := store.AciContext{
			SubscriptionID: subscriptionID,
			Location:       location,
			ResourceGroup:  resourceGroupName,
		}
		createStorageAccount(aciContext, testStorageAccountName)
		defer deleteStorageAccount(aciContext)
		keys := getStorageKeys(aciContext, testStorageAccountName)
		firstKey := *keys[0].Value
		credential, u := createFileShare(firstKey, testShareName)
		uploadFile(credential, u.String(), testFileName, testFileContent)

		mountTarget := "/usr/share/nginx/html"
		output := s.NewDockerCommand("run", "-d", "nginx",
			"-v", fmt.Sprintf("%s:%s@%s:%s",
				testStorageAccountName, firstKey, testShareName, mountTarget),
			"-p", "80:80",
			"--name", testContainerName).ExecOrDie()
		Expect(output).To(ContainSubstring(testContainerName))
		output = s.NewDockerCommand("ps").ExecOrDie()
		lines := Lines(output)
		Expect(len(lines)).To(Equal(2))

		containerFields := Columns(lines[1])
		Expect(containerFields[1]).To(Equal("nginx"))
		Expect(containerFields[2]).To(Equal("Running"))
		exposedIP := containerFields[3]
		containerID := containerFields[0]
		Expect(exposedIP).To(ContainSubstring(":80->80/tcp"))

		nginxExposedURL = strings.ReplaceAll(exposedIP, "->80/tcp", "")
		output = s.NewCommand("curl", nginxExposedURL).ExecOrDie()
		Expect(output).To(ContainSubstring(testFileContent))

		output = s.NewDockerCommand("logs", containerID).ExecOrDie()
		Expect(output).To(ContainSubstring("GET"))
	})

	s.T().Run("follow logs from nginx", func(t *testing.T) {
		timeChan := make(chan time.Time)

		ctx := s.NewDockerCommand("logs", "--follow", testContainerName).WithTimeout(timeChan)
		outChan := make(chan string)

		go func() {
			output, _ := ctx.Exec()
			outChan <- output
		}()

		s.NewCommand("curl", nginxExposedURL+"/test").ExecOrDie()
		// Give the `logs --follow` a little time to get logs of the curl call
		time.Sleep(10 * time.Second)
		// Trigger a timeout to make ctx.Exec exit
		timeChan <- time.Now()

		output := <-outChan

		Expect(output).To(ContainSubstring("/test"))
	})

	s.T().Run("removes container nginx", func(t *testing.T) {
		output := s.NewDockerCommand("rm", testContainerName).ExecOrDie()
		Expect(Lines(output)[0]).To(Equal(testContainerName))
	})

	s.T().Run("re-run nginx with modified cpu/mem, and without --detach and follow logs", func(t *testing.T) {
		shutdown := make(chan time.Time)
		errs := make(chan error)
		outChan := make(chan string)
		cmd := s.NewDockerCommand("run", "nginx", "--memory", "0.1G", "--cpus", "0.1", "-p", "80:80", "--name", testContainerName).WithTimeout(shutdown)
		go func() {
			output, err := cmd.Exec()
			outChan <- output
			errs <- err
		}()
		var containerID string
		err := WaitFor(time.Second, 100*time.Second, errs, func() bool {
			output := s.NewDockerCommand("ps").ExecOrDie()
			lines := Lines(output)
			if len(lines) != 2 {
				return false
			}
			containerFields := Columns(lines[1])
			if containerFields[2] != "Running" {
				return false
			}
			containerID = containerFields[0]
			nginxExposedURL = strings.ReplaceAll(containerFields[3], "->80/tcp", "")
			return true
		})
		Expect(err).NotTo(HaveOccurred())

		s.NewCommand("curl", nginxExposedURL+"/test").ExecOrDie()
		inspect := s.NewDockerCommand("inspect", containerID).ExecOrDie()
		Expect(inspect).To(ContainSubstring("\"CPULimit\": 0.1"))
		Expect(inspect).To(ContainSubstring("\"MemoryLimit\": 107374182"))

		// Give a little time to get logs of the curl call
		time.Sleep(5 * time.Second)
		// Kill
		close(shutdown)

		output := <-outChan
		Expect(output).To(ContainSubstring("/test"))
	})

	s.T().Run("removes container nginx", func(t *testing.T) {
		output := s.NewDockerCommand("rm", testContainerName).ExecOrDie()
		Expect(Lines(output)[0]).To(Equal(testContainerName))
	})

	var exposedURL string
	const composeFile = "../composefiles/aci-demo/aci_demo_port.yaml"
	const composeFileMultiplePorts = "../composefiles/aci-demo/aci_demo_multi_port.yaml"
	const serverContainer = "acidemo_web"
	const wordsContainer = "acidemo_words"

	s.T().Run("deploys a compose app", func(t *testing.T) {
		s.NewDockerCommand("compose", "up", "-f", composeFile, "--project-name", "acidemo").ExecOrDie()
		output := s.NewDockerCommand("ps").ExecOrDie()
		Lines := Lines(output)
		Expect(len(Lines)).To(Equal(4))
		webChecked := false

		for _, line := range Lines[1:] {
			Expect(line).To(ContainSubstring("Running"))
			if strings.Contains(line, serverContainer) {
				webChecked = true
				containerFields := Columns(line)
				exposedIP := containerFields[3]
				Expect(exposedIP).To(ContainSubstring(":80->80/tcp"))

				exposedURL = strings.ReplaceAll(exposedIP, "->80/tcp", "")
				output = s.NewCommand("curl", exposedURL).ExecOrDie()
				Expect(output).To(ContainSubstring("Docker Compose demo"))
				output = s.NewCommand("curl", exposedURL+"/words/noun").ExecOrDie()
				Expect(output).To(ContainSubstring("\"word\":"))
			}
		}

		Expect(webChecked).To(BeTrue())
	})

	s.T().Run("get logs from web service", func(t *testing.T) {
		output := s.NewDockerCommand("logs", serverContainer).ExecOrDie()
		Expect(output).To(ContainSubstring("Listening on port 80"))
	})

	s.T().Run("updates a compose app", func(t *testing.T) {
		s.NewDockerCommand("compose", "up", "-f", composeFileMultiplePorts, "--project-name", "acidemo").ExecOrDie()
		// Expect(output).To(ContainSubstring("Successfully deployed"))
		output := s.NewDockerCommand("ps").ExecOrDie()
		Lines := Lines(output)
		Expect(len(Lines)).To(Equal(4))
		webChecked := false
		wordsChecked := false

		for _, line := range Lines[1:] {
			Expect(line).To(ContainSubstring("Running"))
			if strings.Contains(line, serverContainer) {
				webChecked = true
				containerFields := Columns(line)
				exposedIP := containerFields[3]
				Expect(exposedIP).To(ContainSubstring(":80->80/tcp"))

				url := strings.ReplaceAll(exposedIP, "->80/tcp", "")
				Expect(exposedURL).To(Equal(url))
			}
			if strings.Contains(line, wordsContainer) {
				wordsChecked = true
				containerFields := Columns(line)
				exposedIP := containerFields[3]
				Expect(exposedIP).To(ContainSubstring(":8080->8080/tcp"))

				url := strings.ReplaceAll(exposedIP, "->8080/tcp", "")
				output = s.NewCommand("curl", url+"/noun").ExecOrDie()
				Expect(output).To(ContainSubstring("\"word\":"))
			}
		}

		Expect(webChecked).To(BeTrue())
		Expect(wordsChecked).To(BeTrue())
	})

	s.T().Run("shutdown compose app", func(t *testing.T) {
		s.NewDockerCommand("compose", "down", "--project-name", "acidemo").ExecOrDie()
	})

	s.T().Run("switches back to default context", func(t *testing.T) {
		output := s.NewCommand("docker", "context", "use", "default").ExecOrDie()
		Expect(output).To(ContainSubstring("default"))
	})

	s.T().Run("deletes test context", func(t *testing.T) {
		output := s.NewCommand("docker", "context", "rm", contextName).ExecOrDie()
		Expect(output).To(ContainSubstring(contextName))
	})
}

func createStorageAccount(aciContext store.AciContext, accountName string) azure_storage.Account {
	log.Println("Creating storage account " + accountName)
	storageAccount, err := storage.CreateStorageAccount(context.TODO(), aciContext, accountName)
	Expect(err).To(BeNil())
	Expect(*storageAccount.Name).To(Equal(accountName))
	return storageAccount
}

func getStorageKeys(aciContext store.AciContext, storageAccountName string) []azure_storage.AccountKey {
	list, err := storage.ListKeys(context.TODO(), aciContext, storageAccountName)
	Expect(err).To(BeNil())
	Expect(list.Keys).ToNot(BeNil())
	Expect(len(*list.Keys)).To(BeNumerically(">", 0))

	return *list.Keys
}

func deleteStorageAccount(aciContext store.AciContext) {
	log.Println("Deleting storage account " + testStorageAccountName)
	_, err := storage.DeleteStorageAccount(context.TODO(), aciContext, testStorageAccountName)
	Expect(err).To(BeNil())
}

func createFileShare(key, shareName string) (azfile.SharedKeyCredential, url.URL) {
	// Create a ShareURL object that wraps a soon-to-be-created share's URL and a default pipeline.
	u, _ := url.Parse(fmt.Sprintf("https://%s.file.core.windows.net/%s", testStorageAccountName, shareName))
	credential, err := azfile.NewSharedKeyCredential(testStorageAccountName, key)
	Expect(err).To(BeNil())

	shareURL := azfile.NewShareURL(*u, azfile.NewPipeline(credential, azfile.PipelineOptions{}))
	_, err = shareURL.Create(context.TODO(), azfile.Metadata{}, 0)
	Expect(err).To(BeNil())

	return *credential, *u
}

func uploadFile(credential azfile.SharedKeyCredential, baseURL, fileName, fileContent string) {
	fURL, err := url.Parse(baseURL + "/" + fileName)
	Expect(err).To(BeNil())
	fileURL := azfile.NewFileURL(*fURL, azfile.NewPipeline(&credential, azfile.PipelineOptions{}))
	err = azfile.UploadBufferToAzureFile(context.TODO(), []byte(fileContent), fileURL, azfile.UploadToAzureFileOptions{})
	Expect(err).To(BeNil())
}

func TestE2eACI(t *testing.T) {
	suite.Run(t, new(E2eACISuite))
}

func setupTestResourceGroup(groupName string) {
	log.Println("Creating resource group " + resourceGroupName)
	ctx := context.TODO()
	helper := azure.NewACIResourceGroupHelper()
	models, err := helper.GetSubscriptionIDs(ctx)
	Expect(err).To(BeNil())
	_, err = helper.CreateOrUpdate(ctx, *models[0].SubscriptionID, groupName, resources.Group{
		Location: to.StringPtr(location),
	})
	Expect(err).To(BeNil())
}

func deleteResourceGroup(groupName string) {
	log.Println("Deleting resource group " + resourceGroupName)
	ctx := context.TODO()
	helper := azure.NewACIResourceGroupHelper()
	models, err := helper.GetSubscriptionIDs(ctx)
	Expect(err).To(BeNil())
	err = helper.DeleteAsync(ctx, *models[0].SubscriptionID, groupName)
	Expect(err).To(BeNil())
}

func RandStringBytes(n int) string {
	rand.Seed(time.Now().UnixNano())
	const digits = "0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[rand.Intn(len(digits))]
	}
	return string(b)
}
