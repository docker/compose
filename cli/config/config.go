/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

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
	if name == "default" {
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
	return errors.Wrap(err, "unable to unmarshal config")
}

func configFilePath(dir string) string {
	return filepath.Join(dir, ConfigFileName)
}

// File contains the current context from the docker configuration file
type File struct {
	CurrentContext string `json:"currentContext,omitempty"`
}
