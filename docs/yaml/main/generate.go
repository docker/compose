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
	"github.com/docker/compose/v2/cmd/compose"
	"github.com/spf13/cobra"
)

func generateCliYaml(opts *options) error {
	cmd := &cobra.Command{Use: "docker"}
	cmd.AddCommand(compose.RootCommand(nil))
	disableFlagsInUseLine(cmd)

	cmd.DisableAutoGenTag = true
	return clidocstool.GenYamlTree(cmd, opts.target)
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
	source string
	target string
}

func main() {
	cwd, _ := os.Getwd()
	opts := &options{
		source: cwd,
		target: filepath.Join(cwd, "docs", "reference"),
	}
	fmt.Printf("Project root: %s\n", opts.source)
	fmt.Printf("Generating yaml files into %s\n", opts.target)
	if err := generateCliYaml(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate yaml files: %s\n", err.Error())
	}
}
