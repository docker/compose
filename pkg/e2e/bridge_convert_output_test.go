//go:build e2e

/*
   Copyright 2026 Docker Compose CLI authors

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
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// Regression tests for a captain-reported P1: `bridge convert -o <dir>` used
// to os.RemoveAll the output directory unconditionally, discarding the
// error, so pointing -o at a non-empty directory (e.g. -o . or -o $PWD)
// silently destroyed its content.

// setupNonEmptyOutputDir creates dir/out pre-populated with a guard file, so
// bridge convert sees it as a non-empty output directory.
func setupNonEmptyOutputDir(t *testing.T, s *Scenario, filename, content string) (outDir, guardFile string) {
	t.Helper()
	outDir = filepath.Join(s.Dir(), "out")
	assert.NilError(t, os.MkdirAll(outDir, 0o755))
	guardFile = filepath.Join(outDir, filename)
	assert.NilError(t, os.WriteFile(guardFile, []byte(content), 0o600))
	return outDir, guardFile
}

func TestBridgeConvertOutputNotEmptyDeclined(t *testing.T) {
	s := NewScenario(t, "bridge convert must not touch a non-empty output directory unless the user confirms")
	outDir, guardFile := setupNonEmptyOutputDir(t, s, "README.md", "do not delete me")

	s.Step("convert without confirmation leaves the non-empty output directory untouched",
		ComposeCmd("bridge", "convert", "--output", outDir,
			"--transformation", fmt.Sprintf("docker/compose-bridge-kubernetes:%s", bridgeImageVersion)).MayFail(),
		ExitCode(1),
		FileContains(guardFile, "do not delete me"))
}

func TestBridgeConvertOutputNotEmptyConfirmed(t *testing.T) {
	s := NewScenario(t, "bridge convert must overwrite a non-empty output directory once the user confirms")
	outDir, guardFile := setupNonEmptyOutputDir(t, s, "stale.txt", "stale content")

	s.Step("convert --yes wipes the stale content and regenerates the output",
		ComposeCmd("bridge", "convert", "--output", outDir, "--yes",
			"--transformation", fmt.Sprintf("docker/compose-bridge-kubernetes:%s", bridgeImageVersion)),
		FileAbsent(guardFile),
		FileExists(filepath.Join(outDir, "base", "0-"+s.Project()+"-namespace.yaml")))
}
