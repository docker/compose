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
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	upCmd := &cobra.Command{
		Use:  "up",
		Run:  up,
		Args: cobra.ExactArgs(1),
	}
	upCmd.Flags().String("type", "", "Database type (mysql, postgres, etc.)")
	_ = upCmd.MarkFlagRequired("type")
	upCmd.Flags().Int("size", 10, "Database size in GB")
	upCmd.Flags().String("name", "", "Name of the database to be created")
	_ = upCmd.MarkFlagRequired("name")

	downCmd := &cobra.Command{
		Use:  "down",
		Run:  down,
		Args: cobra.ExactArgs(1),
	}
	downCmd.Flags().String("name", "", "Name of the database to be deleted")
	_ = downCmd.MarkFlagRequired("name")

	c.AddCommand(upCmd, downCmd)
	c.AddCommand(metadataCommand(upCmd, downCmd))
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

func metadataCommand(upCmd, downCmd *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use: "metadata",
		Run: func(cmd *cobra.Command, _ []string) {
			metadata(upCmd, downCmd)
		},
		Args: cobra.NoArgs,
	}
}

func metadata(upCmd, downCmd *cobra.Command) {
	metadata := ProviderMetadata{}
	metadata.Description = "Manage services on AwesomeCloud"
	metadata.Up = commandParameters(upCmd)
	metadata.Down = commandParameters(downCmd)
	jsonMetadata, err := json.Marshal(metadata)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(jsonMetadata))
}

func commandParameters(cmd *cobra.Command) CommandMetadata {
	cmdMetadata := CommandMetadata{}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_, isRequired := f.Annotations[cobra.BashCompOneRequiredFlag]
		cmdMetadata.Parameters = append(cmdMetadata.Parameters, Metadata{
			Name:        f.Name,
			Description: f.Usage,
			Required:    isRequired,
			Type:        f.Value.Type(),
			Default:     f.DefValue,
		})
	})
	return cmdMetadata
}

type ProviderMetadata struct {
	Description string          `json:"description"`
	Up          CommandMetadata `json:"up"`
	Down        CommandMetadata `json:"down"`
}

type CommandMetadata struct {
	Parameters []Metadata `json:"parameters"`
}

type Metadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
}
