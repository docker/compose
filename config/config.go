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

package config

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/docker/compose-cli/context/store"
)

type dirKey struct{}

// WithDir sets the config directory path in the context
func WithDir(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, dirKey{}, path)
}

// Dir returns the config directory path
func Dir(ctx context.Context) string {
	cd, _ := ctx.Value(dirKey{}).(string)
	return cd
}

// LoadFile loads the docker configuration
func LoadFile(dir string) (*File, error) {
	f := &File{}
	err := loadFile(configFilePath(dir), &f)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// WriteCurrentContext writes the selected current context to the Docker
// configuration file. Note, the validity of the context is not checked.
func WriteCurrentContext(dir string, name string) error {
	m := map[string]interface{}{}
	path := configFilePath(dir)
	err := loadFile(path, &m)
	if err != nil {
		return err
	}
	// Match existing CLI behavior
	if name == store.DefaultContextName {
		delete(m, currentContextKey)
	} else {
		m[currentContextKey] = name
	}
	return writeFile(path, m)
}

func writeFile(path string, content map[string]interface{}) error {
	d, err := json.MarshalIndent(content, "", "\t")
	if err != nil {
		return errors.Wrap(err, "unable to marshal config")
	}
	err = ioutil.WriteFile(path, d, 0644)
	return errors.Wrap(err, "unable to write config file")
}

func loadFile(path string, dest interface{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Not an error if there is no config, we're just using defaults
			return nil
		}
		return errors.Wrap(err, "unable to read config file")
	}
	err = json.Unmarshal(data, dest)
	return errors.Wrap(err, "unable to unmarshal config file "+path)
}

func configFilePath(dir string) string {
	return filepath.Join(dir, ConfigFileName)
}

// File contains the current context from the docker configuration file
type File struct {
	CurrentContext string `json:"currentContext,omitempty"`
}
