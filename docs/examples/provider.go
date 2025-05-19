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
	"time"

	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Short: "Compose Provider Example",
		Use:   "demo",
	}
	cmd.AddCommand(composeCommand())
	err := cmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func composeCommand() *cobra.Command {
	c := &cobra.Command{
		Use:              "compose EVENT",
		TraverseChildren: true,
	}
	c.PersistentFlags().String("project-name", "", "compose project name") // unused
	c.AddCommand(&cobra.Command{
		Use:  "up",
		Run:  up,
		Args: cobra.ExactArgs(1),
	})
	c.AddCommand(&cobra.Command{
		Use:  "down",
		Run:  down,
		Args: cobra.ExactArgs(1),
	})
	return c
}

const lineSeparator = "\n"

func up(_ *cobra.Command, args []string) {
	servicename := args[0]
	fmt.Printf(`{ "type": "debug", "message": "Starting %s" }%s`, servicename, lineSeparator)

	for i := 0; i < 100; i += 10 {
		time.Sleep(1 * time.Second)
		fmt.Printf(`{ "type": "info", "message": "Processing ... %d%%" }%s`, i, lineSeparator)
	}
	fmt.Printf(`{ "type": "setenv", "message": "URL=https://magic.cloud/%s" }%s`, servicename, lineSeparator)
}

func down(_ *cobra.Command, _ []string) {
	fmt.Printf(`{ "type": "error", "message": "Permission error" }%s`, lineSeparator)
}
