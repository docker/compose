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

package compose

import (
	"encoding/json"
	"io"
	"os"

	"github.com/docker/cli/cli-plugins/hooks"
	"github.com/docker/cli/cli-plugins/metadata"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v5/cmd/formatter"
)

const deepLink = "docker-desktop://dashboard/logs"

func composeLogsHint() string {
	return "Filter, search, and stream logs from all your Compose services\nin one place with Docker Desktop's Logs view. " + hintLink(deepLink)
}

func dockerLogsHint() string {
	return "View and search logs for all containers in one place\nwith Docker Desktop's Logs view. " + hintLink(deepLink)
}

// hintLink returns a clickable OSC 8 terminal hyperlink when ANSI is allowed,
// or the plain URL when ANSI output is suppressed via NO_COLOR or COMPOSE_ANSI.
func hintLink(url string) string {
	if shouldDisableAnsi() {
		return url
	}
	return formatter.OSC8Link(url, url)
}

// shouldDisableAnsi checks whether ANSI escape sequences should be explicitly
// suppressed via environment variables. The hook runs as a separate subprocess
// where the normal PersistentPreRunE (which calls formatter.SetANSIMode) is
// skipped, so we check NO_COLOR and COMPOSE_ANSI directly.
//
// TTY detection is intentionally omitted: the hook produces a JSON response
// whose template is rendered by the parent Docker CLI process via
// PrintNextSteps (which itself emits bold ANSI unconditionally). The hook
// subprocess cannot reliably detect whether the parent's output is a terminal.
func shouldDisableAnsi() bool {
	if noColor, ok := os.LookupEnv("NO_COLOR"); ok && noColor != "" {
		return true
	}
	if v, ok := os.LookupEnv("COMPOSE_ANSI"); ok && v == formatter.Never {
		return true
	}
	return false
}

// hookHint defines a hint that can be returned by the hooks handler.
// When checkFlags is nil, the hint is always returned for the matching command.
// When checkFlags is set, the hint is only returned if the check passes.
type hookHint struct {
	template   func() string
	checkFlags func(flags map[string]string) bool
}

// hooksHints maps hook root commands to their hint definitions.
var hooksHints = map[string]hookHint{
	// standalone "docker logs" (not a compose subcommand)
	"logs":         {template: dockerLogsHint},
	"compose logs": {template: composeLogsHint},
	"compose up": {
		template: composeLogsHint,
		checkFlags: func(flags map[string]string) bool {
			// Only show the hint when running in detached mode
			_, hasDetach := flags["detach"]
			_, hasD := flags["d"]
			return hasDetach || hasD
		},
	},
}

// HooksCommand returns the hidden subcommand that the Docker CLI invokes
// after command execution when the compose plugin has hooks configured.
// Docker Desktop is responsible for registering which commands trigger hooks
// and for gating on feature flags/settings — the hook handler simply
// responds with the appropriate hint message.
func HooksCommand() *cobra.Command {
	return &cobra.Command{
		Use:    metadata.HookSubcommandName,
		Hidden: true,
		// Override PersistentPreRunE to prevent the parent's PersistentPreRunE
		// (plugin initialization) from running for hook invocations.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleHook(args, cmd.OutOrStdout())
		},
	}
}

func handleHook(args []string, w io.Writer) error {
	if len(args) == 0 {
		return nil
	}

	var hookData hooks.Request
	if err := json.Unmarshal([]byte(args[0]), &hookData); err != nil {
		return nil
	}

	hint, ok := hooksHints[hookData.RootCmd]
	if !ok {
		return nil
	}

	if hint.checkFlags != nil && !hint.checkFlags(hookData.Flags) {
		return nil
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(hooks.Response{
		Type:     hooks.NextSteps,
		Template: hint.template(),
	})
}
