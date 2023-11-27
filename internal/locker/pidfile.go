/*
   Copyright 2023 Docker Compose CLI authors

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

package locker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/docker/pkg/pidfile"
)

type Pidfile struct {
	path string
}

func NewPidfile(projectName string) (*Pidfile, error) {
	run, err := runDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(run, fmt.Sprintf("%s.pid", projectName))
	return &Pidfile{path: path}, nil
}

func (f *Pidfile) Lock() error {
	return pidfile.Write(f.path, os.Getpid())
}
