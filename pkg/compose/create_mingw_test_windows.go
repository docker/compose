//go:build windows
// +build windows

package compose

import (
	"bytes"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
)

func TestBuildMountGitBashWarning(t *testing.T) {
	oldOut := logrus.StandardLogger().Out
	var buf bytes.Buffer
	logrus.SetOutput(&buf)
	defer logrus.SetOutput(oldOut)

	vol := types.ServiceVolumeConfig{
		Type:   types.VolumeTypeBind,
		Source: "work-folder;C",
		Target: `C:\\Program Files\\Git\\usr\\bin`,
	}
	_, err := buildMount(types.Project{}, vol)
	if err != nil {
		t.Fatalf("buildMount error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("Git Bash mangled your volume path")) {
		t.Fatalf("expected warning about Git Bash mangling, got: %s", buf.String())
	}
}
