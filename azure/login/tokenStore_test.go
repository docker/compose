package login

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
)

type tokenStoreTestSuite struct {
	suite.Suite
}

func (suite *tokenStoreTestSuite) TestCreateStoreFromExistingFolder() {
	existingDir, err := ioutil.TempDir("", "test_store")
	Expect(err).To(BeNil())

	storePath := filepath.Join(existingDir, tokenStoreFilename)
	store, err := newTokenStore(storePath)
	Expect(err).To(BeNil())
	Expect((store.filePath)).To(Equal(storePath))
}

func (suite *tokenStoreTestSuite) TestCreateStoreFromNonExistingFolder() {
	existingDir, err := ioutil.TempDir("", "test_store")
	Expect(err).To(BeNil())

	storePath := filepath.Join(existingDir, "new", tokenStoreFilename)
	store, err := newTokenStore(storePath)
	Expect(err).To(BeNil())
	Expect((store.filePath)).To(Equal(storePath))

	newDir, err := os.Stat(filepath.Join(existingDir, "new"))
	Expect(err).To(BeNil())
	Expect(newDir.Mode().IsDir()).To(BeTrue())
}

func (suite *tokenStoreTestSuite) TestErrorIfParentFolderIsAFile() {
	existingDir, err := ioutil.TempFile("", "test_store")
	Expect(err).To(BeNil())

	storePath := filepath.Join(existingDir.Name(), tokenStoreFilename)
	_, err = newTokenStore(storePath)
	Expect(err).To(MatchError(errors.New("cannot use path " + storePath + " ; " + existingDir.Name() + " already exists and is not a directory")))
}

func TestTokenStoreSuite(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(tokenStoreTestSuite))
}
