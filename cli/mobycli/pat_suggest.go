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

package mobycli

import (
	"fmt"
	"os"
	"strings"

	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/registry"
	"github.com/hashicorp/go-uuid"
)

const (
	// patSuggestMsg is a message to suggest the use of PAT (personal access tokens).
	patSuggestMsg = `Logging in with your password grants your terminal complete access to your account. 
For better security, log in with a limited-privilege personal access token. Learn more at https://docs.docker.com/go/access-tokens/`

	// patPrefix represents a docker personal access token prefix.
	patPrefix = "dckrp_"
)

// displayPATSuggestMsg displays a message suggesting users to use PATs instead of passwords to reduce scope.
func displayPATSuggestMsg(cmdArgs []string) {
	if os.Getenv("DOCKER_PAT_SUGGEST") == "false" {
		return
	}
	if !isUsingDefaultRegistry(cmdArgs) {
		return
	}
	authCfg, err := config.LoadDefaultConfigFile(os.Stderr).GetAuthConfig(registry.IndexServer)
	if err != nil {
		return
	}
	if !isUsingPassword(authCfg.Password) {
		return
	}
	fmt.Fprintf(os.Stderr, "\n"+patSuggestMsg+"\n")
}

func isUsingDefaultRegistry(cmdArgs []string) bool {
	for i := 1; i < len(cmdArgs); i++ {
		if strings.HasPrefix(cmdArgs[i], "-") {
			i++
			continue
		}
		return cmdArgs[i] == registry.IndexServer
	}
	return true
}

func isUsingPassword(pass string) bool {
	if pass == "" { // ignore if no password (or SSO)
		return false
	}
	if _, err := uuid.ParseUUID(pass); err == nil {
		return false
	}
	if strings.HasPrefix(pass, patPrefix) {
		return false
	}
	return true
}
