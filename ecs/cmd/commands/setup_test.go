package commands

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/docker/cli/cli/config"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/fs"
	"gotest.tools/v3/golden"
)

func TestDefaultAwsContextName(t *testing.T) {
	dir := fs.NewDir(t, "setup")
	defer dir.Remove()
	cmd := NewRootCmd(nil)
	dockerConfig := config.Dir()
	config.SetDir(dir.Path())
	defer config.SetDir(dockerConfig)

	cmd.SetArgs([]string{"setup", "--cluster", "clusterName", "--profile", "profileName", "--region", "regionName"})
	err := cmd.Execute()
	assert.NilError(t, err)

	files, err := filepath.Glob(dir.Join("contexts", "meta", "*", "meta.json"))
	assert.NilError(t, err)
	assert.Equal(t, len(files), 1)
	b, err := ioutil.ReadFile(files[0])
	assert.NilError(t, err)
	golden.Assert(t, string(b), "context.golden")
}
