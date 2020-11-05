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

package context

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/cli/mobycli"
	apicontext "github.com/docker/compose-cli/context"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/formatter"
)

type lsOpts struct {
	quiet  bool
	json   bool
	format string
}

func (o lsOpts) validate() error {
	if o.quiet && o.json {
		return errors.New(`cannot combine "quiet" and "json" options`)
	}
	return nil
}

func listCommand() *cobra.Command {
	var opts lsOpts
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List available contexts",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.quiet, "quiet", "q", false, "Only show context names")
	cmd.Flags().StringVar(&opts.format, "format", "", "Format the output. Values: [pretty | json]. (Default: pretty)")

	return cmd
}

func runList(cmd *cobra.Command, opts lsOpts) error {
	err := opts.validate()
	if err != nil {
		return err
	}
	format := strings.ToLower(strings.ReplaceAll(opts.format, " ", ""))
	if format != "" && format != formatter.JSON && format != formatter.PRETTY && format != formatter.TemplateLegacyJSON {
		mobycli.Exec(cmd.Root())
		return nil
	}

	ctx := cmd.Context()
	currentContext := apicontext.CurrentContext(ctx)
	s := store.ContextStore(ctx)
	contexts, err := s.List()
	if err != nil {
		return err
	}

	sort.Slice(contexts, func(i, j int) bool {
		return strings.Compare(contexts[i].Name, contexts[j].Name) == -1
	})

	if opts.quiet {
		for _, c := range contexts {
			fmt.Println(c.Name)
		}
		return nil
	}

	if opts.json || format == formatter.JSON {
		opts.format = formatter.JSON
	}
	if format == formatter.TemplateLegacyJSON {
		opts.format = formatter.TemplateLegacyJSON
	}

	view := viewFromContextList(contexts, currentContext)
	return formatter.Print(view, opts.format, os.Stdout,
		func(w io.Writer) {
			for _, c := range view {
				contextName := c.Name
				if c.Current {
					contextName += " *"
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					contextName,
					c.ContextType,
					c.Description,
					c.DockerEndpoint,
					c.KubernetesEndpoint,
					c.StackOrchestrator)
			}
		},
		"NAME", "TYPE", "DESCRIPTION", "DOCKER ENDPOINT", "KUBERNETES ENDPOINT", "ORCHESTRATOR")
}

func getEndpoint(name string, meta map[string]interface{}) string {
	endpoints, ok := meta[name]
	if !ok {
		return ""
	}
	data, ok := endpoints.(*store.Endpoint)
	if !ok {
		return ""
	}

	result := data.Host
	if data.DefaultNamespace != "" {
		result += fmt.Sprintf(" (%s)", data.DefaultNamespace)
	}

	return result
}

type contextView struct {
	Current            bool
	Description        string
	DockerEndpoint     string
	KubernetesEndpoint string
	ContextType        string
	Name               string
	StackOrchestrator  string
}

func viewFromContextList(contextList []*store.DockerContext, currentContext string) []contextView {
	retList := make([]contextView, len(contextList))
	for i, c := range contextList {
		retList[i] = contextView{
			Current:            c.Name == currentContext,
			Description:        c.Metadata.Description,
			DockerEndpoint:     getEndpoint("docker", c.Endpoints),
			KubernetesEndpoint: getEndpoint("kubernetes", c.Endpoints),
			Name:               c.Name,
			ContextType:        c.Type(),
			StackOrchestrator:  c.Metadata.StackOrchestrator,
		}
	}
	return retList
}
