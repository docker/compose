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
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/docker/cli/cli-plugins/hooks"
	"github.com/docker/cli/cli-plugins/metadata"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v5/cmd/formatter"
	"github.com/docker/compose/v5/internal/desktop"
)

func composeLogsHint(appID string) string {
	return "Filter, search, and stream logs from all your Compose services\nin one place with Docker Desktop's Logs view. " + hintLink(desktop.BuildLogsURL(appID))
}

func dockerLogsHint(appID string) string {
	return "View and search logs for all containers in one place\nwith Docker Desktop's Logs view. " + hintLink(desktop.BuildLogsURL(appID))
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

type hookHint struct {
	template       func(appID string) string
	checkFlags     func(flags map[string]string) bool
	resolveProject bool
}

var hooksHints = map[string]hookHint{
	// "docker logs": the CLI hook payload doesn't carry the positional
	// container id, so the link is emitted unfiltered.
	"logs":         {template: dockerLogsHint},
	"compose logs": {template: composeLogsHint, resolveProject: true},
	"compose up": {
		template:       composeLogsHint,
		resolveProject: true,
		checkFlags: func(flags map[string]string) bool {
			return hasFlag(flags, "detach", "d")
		},
	},
}

// Test seams. Replace via t.Cleanup; not safe to mutate from t.Parallel().
var (
	logsTabEnabled = func(ctx context.Context) bool {
		return desktop.IsFeatureActiveStandalone(ctx, desktop.FeatureLogsTab)
	}
	resolveAppID = defaultResolveAppID
)

const projectNameResolveTimeout = 250 * time.Millisecond

// Root-command flags whose values change which project the loader would
// resolve. The hook payload exposes flag names but not values, so when any
// is set we skip the appId rather than emit a wrong filter. workdir is the
// deprecated alias for --project-directory; env-file can set
// COMPOSE_PROJECT_NAME via the .env file it points at.
var projectScopingFlags = []string{
	"project-name", "p",
	"file", "f",
	"project-directory", "workdir",
	"env-file",
}

func defaultResolveAppID(ctx context.Context, flags map[string]string) string {
	workDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return resolveAppIDIn(ctx, flags, workDir)
}

// Split from defaultResolveAppID so tests can pass a t.TempDir() instead
// of mutating process state via t.Chdir.
func resolveAppIDIn(ctx context.Context, flags map[string]string, workDir string) string {
	if hasFlag(flags, projectScopingFlags...) {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, projectNameResolveTimeout)
	defer cancel()

	opts, err := cli.NewProjectOptions(nil,
		cli.WithWorkingDirectory(workDir),
		cli.WithOsEnv,
		cli.WithDotEnv,
		cli.WithConfigFileEnv,
		cli.WithDefaultConfigPath,
	)
	if err != nil {
		return ""
	}
	project, err := opts.LoadProject(ctx)
	if err != nil {
		return ""
	}
	return project.Name
}

func hasFlag(flags map[string]string, names ...string) bool {
	for _, n := range names {
		if _, ok := flags[n]; ok {
			return true
		}
	}
	return false
}

// HooksCommand returns the hidden subcommand that the Docker CLI invokes
// after command execution when the compose plugin has hooks configured.
// Docker Desktop is responsible for registering which commands trigger hooks
// in the docker CLI config; the handler gates all hints on the LogsTab
// feature flag before emitting them.
func HooksCommand() *cobra.Command {
	return &cobra.Command{
		Use:    metadata.HookSubcommandName,
		Hidden: true,
		// Override PersistentPreRunE to prevent the parent's PersistentPreRunE
		// (plugin initialization) from running for hook invocations.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleHook(cmd.Context(), args, cmd.OutOrStdout())
		},
	}
}

func handleHook(ctx context.Context, args []string, w io.Writer) error {
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

	if !logsTabEnabled(ctx) {
		return nil
	}

	var appID string
	if hint.resolveProject {
		appID = resolveAppID(ctx, hookData.Flags)
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(hooks.Response{
		Type:     hooks.NextSteps,
		Template: hint.template(appID),
	})
}
