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

package watch

import (
	"os"
	"path/filepath"
)

// TempDir holds a temp directory and allows easy access to new temp directories.
type TempDir struct {
	dir string
}

// NewDir creates a new TempDir in the default location (typically $TMPDIR)
func NewDir(prefix string) (*TempDir, error) {
	return NewDirAtRoot("", prefix)
}

// NewDir creates a new TempDir at the given root.
func NewDirAtRoot(root, prefix string) (*TempDir, error) {
	tmpDir, err := os.MkdirTemp(root, prefix)
	if err != nil {
		return nil, err
	}

	realTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		return nil, err
	}

	return &TempDir{dir: realTmpDir}, nil
}

// NewDirAtSlashTmp creates a new TempDir at /tmp
func NewDirAtSlashTmp(prefix string) (*TempDir, error) {
	fullyResolvedPath, err := filepath.EvalSymlinks("/tmp")
	if err != nil {
		return nil, err
	}
	return NewDirAtRoot(fullyResolvedPath, prefix)
}

// d.NewDir creates a new TempDir under d
func (d *TempDir) NewDir(prefix string) (*TempDir, error) {
	d2, err := os.MkdirTemp(d.dir, prefix)
	if err != nil {
		return nil, err
	}
	return &TempDir{d2}, nil
}

func (d *TempDir) NewDeterministicDir(name string) (*TempDir, error) {
	d2 := filepath.Join(d.dir, name)
	err := os.Mkdir(d2, 0o700)
	if os.IsExist(err) {
		return nil, err
	} else if err != nil {
		return nil, err
	}
	return &TempDir{d2}, nil
}

func (d *TempDir) TearDown() error {
	return os.RemoveAll(d.dir)
}

func (d *TempDir) Path() string {
	return d.dir
}

// Possible extensions:
// temp file
// named directories or files (e.g., we know we want one git repo for our object, but it should be temporary)
