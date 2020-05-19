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
	"os"
	"path/filepath"

	"github.com/spf13/pflag"
)

const (
	// ConfigFileName is the name of config file
	ConfigFileName = "config.json"
	// ConfigFileDir is the default folder where the config file is stored
	ConfigFileDir = ".docker"
	// ConfigFlagName is the name of the config flag
	ConfigFlagName = "config"
)

// ConfigFlags are the global CLI flags
// nolint stutter
type ConfigFlags struct {
	Config string
}

// AddConfigFlags adds persistent (global) flags
func (c *ConfigFlags) AddConfigFlags(flags *pflag.FlagSet) {
	flags.StringVar(&c.Config, ConfigFlagName, confDir(), "Location of the client config files `DIRECTORY`")
}

func confDir() string {
	env := os.Getenv("DOCKER_CONFIG")
	if env != "" {
		return env
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ConfigFileDir)
}
