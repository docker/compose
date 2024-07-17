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
	"fmt"
	"os"
	"path/filepath"

	clidocstool "github.com/docker/cli-docs-tool"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/cmd/compose"
	"github.com/spf13/cobra"
)

func generateDocs(opts *options) error {
	dockerCLI, err := command.NewDockerCli()
	if err != nil {
		return err
	}
	cmd := &cobra.Command{
		Use:               "docker",
		DisableAutoGenTag: true,
	}
	cmd.AddCommand(compose.RootCommand(dockerCLI, nil))
	disableFlagsInUseLine(cmd)

	tool, err := clidocstool.New(clidocstool.Options{
		Root:      cmd,
		SourceDir: opts.source,
		TargetDir: opts.target,
		Plugin:    true,
	})
	if err != nil {
		return err
	}
	for _, format := range opts.formats {
		switch format {
		case "yaml":
			if err := tool.GenYamlTree(cmd); err != nil {
				return err
			}
		case "md":
			if err := tool.GenMarkdownTree(cmd); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown format %q", format)
		}
	}
	return nil
}

func disableFlagsInUseLine(cmd *cobra.Command) {
	visitAll(cmd, func(ccmd *cobra.Command) {
		// do not add a `[flags]` to the end of the usage line.
		ccmd.DisableFlagsInUseLine = true
	})
}

// visitAll will traverse all commands from the root.
// This is different from the VisitAll of cobra.Command where only parents
// are checked.
func visitAll(root *cobra.Command, fn func(*cobra.Command)) {
	for _, cmd := range root.Commands() {
		visitAll(cmd, fn)
	}
	fn(root)
}

type options struct {
	source  string
	target  string
	formats []string
}

func main() {
	cwd, _ := os.Getwd()
	opts := &options{
		source:  filepath.Join(cwd, "docs", "reference"),
		target:  filepath.Join(cwd, "docs", "reference"),
		formats: []string{"yaml", "md"},
	}
	fmt.Printf("Project root: %s\n", opts.source)
	fmt.Printf("Generating yaml files into %s\n", opts.target)
	if err := generateDocs(opts); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to generate documentation: %s\n", err.Error())
	}
}
