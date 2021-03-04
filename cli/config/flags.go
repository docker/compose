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
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"

	"github.com/docker/compose-cli/api/config"
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

// GetCurrentContext get current context based on opts, env vars
func GetCurrentContext(contextOpt string, configDir string, hosts []string) string {
	// host and context flags cannot be both set at the same time -- the local backend enforces this when resolving hostname
	// -H flag disables context --> set default as current
	if len(hosts) > 0 {
		return "default"
	}
	// DOCKER_HOST disables context --> set default as current
	if _, present := os.LookupEnv("DOCKER_HOST"); present {
		return "default"
	}
	res := contextOpt
	if res == "" {
		// check if DOCKER_CONTEXT env variable was set
		if _, present := os.LookupEnv("DOCKER_CONTEXT"); present {
			res = os.Getenv("DOCKER_CONTEXT")
		}

		if res == "" {
			config, err := config.LoadFile(configDir)
			if err != nil {
				fmt.Fprintln(os.Stderr, errors.Wrap(err, "WARNING"))
				return "default"
			}
			res = config.CurrentContext
		}
	}
	if res == "" {
		res = "default"
	}
	return res
}
