package storage

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/Azure/azure-storage-file-go/azfile"
	. "github.com/onsi/gomega"

	"github.com/docker/api/azure"
	"github.com/docker/api/context/store"
)

const (
	resourceGroupName = "rgulyssessouza"
	location          = "westeurope"

	testAccountName = "dockertestaccountname"
	testShareName   = "dockertestsharename"
	testContent     = "test content!"
)

func TestGetContainerName(t *testing.T) {
	RegisterTestingT(t)

	subscriptionID, err := azure.GetSubscriptionID(context.TODO())
	Expect(err).To(BeNil())
	aciContext := store.AciContext{
		SubscriptionID: subscriptionID,
		Location:       location,
		ResourceGroup:  resourceGroupName,
	}

	storageAccount, err := CreateStorageAccount(context.TODO(), aciContext, testAccountName)
	Expect(err).To(BeNil())
	Expect(*storageAccount.Name).To(Equal(testAccountName))

	list, err := ListKeys(context.TODO(), aciContext, *storageAccount.Name)
	Expect(err).To(BeNil())

	firstKey := *(*list.Keys)[0].Value

	// Create a ShareURL object that wraps a soon-to-be-created share's URL and a default pipeline.
	u, _ := url.Parse(fmt.Sprintf("https://%s.file.core.windows.net/%s", testAccountName, testShareName))
	credential, err := azfile.NewSharedKeyCredential(testAccountName, firstKey)
	Expect(err).To(BeNil())

	shareURL := azfile.NewShareURL(*u, azfile.NewPipeline(credential, azfile.PipelineOptions{}))
	_, err = shareURL.Create(context.TODO(), azfile.Metadata{}, 0)
	Expect(err).To(BeNil())

	fURL, err := url.Parse(u.String() + "/testfile")
	Expect(err).To(BeNil())
	fileURL := azfile.NewFileURL(*fURL, azfile.NewPipeline(credential, azfile.PipelineOptions{}))
	err = azfile.UploadBufferToAzureFile(context.TODO(), []byte(testContent), fileURL, azfile.UploadToAzureFileOptions{})
	Expect(err).To(BeNil())

	b := make([]byte, len(testContent))
	_, err = azfile.DownloadAzureFileToBuffer(context.TODO(), fileURL, b, azfile.DownloadFromAzureFileOptions{})
	Expect(err).To(BeNil())
	Expect(string(b)).To(Equal(testContent))

	_, err = DeleteStorageAccount(context.TODO(), aciContext, testAccountName)
	Expect(err).To(BeNil())
}
