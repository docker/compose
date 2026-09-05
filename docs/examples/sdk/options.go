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

package main

import (
	"bytes"
	"os"

	"github.com/docker/cli/cli/command"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
)

// customService demonstrates the options accepted by NewComposeService: the
// second fenced Go block of docs/sdk.md is this function's body, verbatim
// (pinned by sdk_md_test.go).
func customService(dockerCLI command.Cli) (api.Compose, error) {
	// Create a custom output buffer to capture logs
	var outputBuffer bytes.Buffer

	// Create a compose service with custom options
	service, err := compose.NewComposeService(dockerCLI,
		compose.WithOutputStream(&outputBuffer),      // Redirect output to custom writer
		compose.WithErrorStream(os.Stderr),           // Use stderr for errors
		compose.WithMaxConcurrency(4),                // Limit concurrent operations
		compose.WithPrompt(compose.AlwaysOkPrompt()), // Auto-confirm all prompts
	)
	return service, err
}
