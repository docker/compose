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
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/docker/compose-cli/cli/cmd/compose"
	. "github.com/docker/compose-cli/docs/yaml"
)

const descriptionSourcePath = "docs/reference/"

func generateCliYaml(opts *options) error {
	cmd := &cobra.Command{Use: "docker"}
	cmd.AddCommand(compose.RootCommand("local", nil))
	disableFlagsInUseLine(cmd)
	source := filepath.Join(opts.source, descriptionSourcePath)
	if err := loadLongDescription(cmd, source); err != nil {
		return err
	}

	cmd.DisableAutoGenTag = true
	return GenYamlTree(cmd, opts.target)
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

func loadLongDescription(cmd *cobra.Command, path ...string) error {
	for _, cmd := range cmd.Commands() {
		if cmd.Name() == "" {
			continue
		}
		fullpath := filepath.Join(path[0], strings.Join(append(path[1:], cmd.Name()), "_")+".md")

		if cmd.HasSubCommands() {
			if err := loadLongDescription(cmd, path[0], cmd.Name()); err != nil {
				return err
			}
		}

		if _, err := os.Stat(fullpath); err != nil {
			log.Printf("WARN: %s does not exist, skipping\n", fullpath)
			continue
		}

		content, err := ioutil.ReadFile(fullpath)
		if err != nil {
			return err
		}
		description, examples := ParseMDContent(string(content))
		cmd.Long = description
		cmd.Example = examples
	}
	return nil
}

type options struct {
	source string
	target string
}

func parseArgs() (*options, error) {
	opts := &options{}
	cwd, _ := os.Getwd()
	flags := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	flags.StringVar(&opts.source, "root", cwd, "Path to project root")
	flags.StringVar(&opts.target, "target", filepath.Join(cwd, "docs", "reference"), "Target path for generated yaml files")
	err := flags.Parse(os.Args[1:])
	return opts, err
}

func main() {
	opts, err := parseArgs()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	fmt.Printf("Project root: %s\n", opts.source)
	fmt.Printf("Generating yaml files into %s\n", opts.target)
	if err := generateCliYaml(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate yaml files: %s\n", err.Error())
	}
}
