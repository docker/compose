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

package context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LoadConfigFile(configDir string, configFileName string) (*ConfigFile, error) {
	filename := filepath.Join(configDir, configFileName)
	configFile := &ConfigFile{
		Filename: filename,
	}

	if _, err := os.Stat(filename); err == nil {
		file, err := os.Open(filename)
		if err != nil {
			return nil, fmt.Errorf("can't read %s: %w", filename, err)
		}
		defer file.Close()
		err = json.NewDecoder(file).Decode(&configFile)
		if err != nil {
			err = fmt.Errorf("can't read %s: %w", filename, err)
		}
		return configFile, err
	} else if !os.IsNotExist(err) {
		// if file is there but we can't stat it for any reason other
		// than it doesn't exist then stop
		return nil, fmt.Errorf("can't read %s: %w", filename, err)
	}
	return configFile, nil
}

type ConfigFile struct {
	Filename       string `json:"-"` // Note: for internal use only
	CurrentContext string `json:"currentContext,omitempty"`
}
