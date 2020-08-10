package secrets

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/fs"
)

func TestRawSecret(t *testing.T) {
	dir := fs.NewDir(t, "secrets").Path()
	os.Setenv("raw", "something_secret")
	defer os.Unsetenv("raw")

	err := CreateSecretFiles(Secret{
		Name: "raw",
		Keys: nil,
	}, dir)
	assert.NilError(t, err)
	file, err := ioutil.ReadFile(filepath.Join(dir, "raw"))
	assert.NilError(t, err)
	content := string(file)
	assert.Equal(t, content, "something_secret")
}

func TestSelectedKeysSecret(t *testing.T) {
	dir := fs.NewDir(t, "secrets").Path()
	os.Setenv("json", `
{
   "foo": "bar",
   "zot": "qix"
}`)
	defer os.Unsetenv("json")

	err := CreateSecretFiles(Secret{
		Name: "json",
		Keys: []string{"foo"},
	}, dir)
	assert.NilError(t, err)
	file, err := ioutil.ReadFile(filepath.Join(dir, "json", "foo"))
	assert.NilError(t, err)
	content := string(file)
	assert.Equal(t, content, "bar")

	_, err = os.Stat(filepath.Join(dir, "json", "zot"))
	assert.Check(t, os.IsNotExist(err))
}

func TestAllKeysSecret(t *testing.T) {
	dir := fs.NewDir(t, "secrets").Path()
	os.Setenv("json", `
{
   "foo": "bar",
   "zot": "qix"
}`)
	defer os.Unsetenv("json")

	err := CreateSecretFiles(Secret{
		Name: "json",
		Keys: []string{"*"},
	}, dir)
	assert.NilError(t, err)
	file, err := ioutil.ReadFile(filepath.Join(dir, "json", "foo"))
	assert.NilError(t, err)
	content := string(file)
	assert.Equal(t, content, "bar")

	file, err = ioutil.ReadFile(filepath.Join(dir, "json", "zot"))
	assert.NilError(t, err)
	content = string(file)
	assert.Equal(t, content, "qix")
}

func TestUnknownSecret(t *testing.T) {
	dir := fs.NewDir(t, "secrets").Path()

	err := CreateSecretFiles(Secret{
		Name: "not_set",
		Keys: nil,
	}, dir)
	assert.Check(t, err != nil)
}
