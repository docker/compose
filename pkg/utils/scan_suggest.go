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

package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	pluginmanager "github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli/command"
	cliConfig "github.com/docker/cli/cli/config"
)

// ScanSuggestMsg display a message after a successful build to suggest use of `docker scan` command
const ScanSuggestMsg = "Use 'docker scan' to run Snyk tests against images to find vulnerabilities and learn how to fix them"

// DisplayScanSuggestMsg displlay a message suggesting users can scan new image
func DisplayScanSuggestMsg() {
	if os.Getenv("DOCKER_SCAN_SUGGEST") == "false" {
		return
	}
	if !scanAvailable() {
		return
	}
	if scanAlreadyInvoked() {
		return
	}
	fmt.Fprintf(os.Stderr, "\n"+ScanSuggestMsg+"\n")
}

func scanAlreadyInvoked() bool {
	filename := filepath.Join(cliConfig.Dir(), "scan", "config.json")
	f, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	if f.IsDir() { // should never happen, do not bother user with suggestion if something goes wrong
		return true
	}
	type scanOptin struct {
		Optin bool `json:"optin"`
	}
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return true
	}
	scanConfig := scanOptin{}
	err = json.Unmarshal(data, &scanConfig)
	if err != nil {
		return true
	}
	return scanConfig.Optin
}

func scanAvailable() bool {
	cli, err := command.NewDockerCli()
	if err != nil {
		return false
	}
	plugins, err := pluginmanager.ListPlugins(cli, nil)
	if err != nil {
		return false
	}
	for _, plugin := range plugins {
		if plugin.Name == "scan" {
			return true
		}
	}
	return false
}
