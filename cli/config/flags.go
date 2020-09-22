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
	"os"
	"path/filepath"

	"github.com/spf13/pflag"

	"github.com/docker/compose-cli/config"
)

// ConfigFlags are the global CLI flags
// nolint stutter
type ConfigFlags struct {
	Config string
}

// AddConfigFlags adds persistent (global) flags
func (c *ConfigFlags) AddConfigFlags(flags *pflag.FlagSet) {
	flags.StringVar(&c.Config, config.ConfigFlagName, confDir(), "Location of the client config files `DIRECTORY`")
}

func confDir() string {
	env := os.Getenv("DOCKER_CONFIG")
	if env != "" {
		return env
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, config.ConfigFileDir)
}
