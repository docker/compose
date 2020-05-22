/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package context

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
)

func listCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List available contexts",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context())
		},
	}
	return cmd
}

func runList(ctx context.Context) error {
	currentContext := apicontext.CurrentContext(ctx)
	s := store.ContextStore(ctx)
	contexts, err := s.List()
	if err != nil {
		return err
	}

	sort.Slice(contexts, func(i, j int) bool {
		return strings.Compare(contexts[i].Name, contexts[j].Name) == -1
	})

	w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tDESCRIPTION\tDOCKER ENPOINT\tKUBERNETES ENDPOINT\tORCHESTRATOR")
	format := "%s\t%s\t%s\t%s\t%s\t%s\n"

	for _, c := range contexts {
		contextName := c.Name
		if c.Name == currentContext {
			contextName += " *"
		}

		fmt.Fprintf(w,
			format,
			contextName,
			c.Metadata.Type,
			c.Metadata.Description,
			getEndpoint("docker", c.Endpoints),
			getEndpoint("kubernetes", c.Endpoints),
			c.Metadata.StackOrchestrator)
	}

	return w.Flush()
}

func getEndpoint(name string, meta map[string]store.Endpoint) string {
	d, ok := meta[name]
	if !ok {
		return ""
	}

	result := d.Host
	if d.DefaultNamespace != "" {
		result += fmt.Sprintf(" (%s)", d.DefaultNamespace)
	}

	return result
}
