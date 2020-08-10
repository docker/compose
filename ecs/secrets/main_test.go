package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/fs"
)

func TestParseSecrets(t *testing.T) {
	secrets := parseInput([]string{
		"foo",
		"bar:*",
		"zot:key0,key1",
	})
	assert.Check(t, len(secrets) == 3)
	assert.Check(t, secrets[0].name == "foo")
	assert.Check(t, secrets[0].keys == nil)

	assert.Check(t, secrets[1].name == "bar")
	assert.Check(t, len(secrets[1].keys) == 1)
	assert.Check(t, secrets[1].keys[0] == "*")

	assert.Check(t, secrets[2].name == "zot")
	assert.Check(t, len(secrets[2].keys) == 2)
	assert.Check(t, secrets[2].keys[0] == "key0")
	assert.Check(t, secrets[2].keys[1] == "key1")
}

func TestRawSecret(t *testing.T) {
	dir := fs.NewDir(t, "secrets").Path()
	os.Setenv("raw", "something_secret")
	defer os.Unsetenv("raw")

	err := createSecretFiles(secret{
		name: "raw",
		keys: nil,
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

	err := createSecretFiles(secret{
		name: "json",
		keys: []string{"foo"},
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

	err := createSecretFiles(secret{
		name: "json",
		keys: []string{"*"},
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

	err := createSecretFiles(secret{
		name: "not_set",
		keys: nil,
	}, dir)
	assert.Check(t, err != nil)
}
