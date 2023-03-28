/*
   Copyright 2023 Docker Compose CLI authors

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
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/spf13/cobra"
)

type vizOptions struct {
	*ProjectOptions
	includeNetworks  bool
	includePorts     bool
	includeImageName bool
	indentationStr   string
}

// maps a service with the services it depends on
type vizGraph map[*types.ServiceConfig][]*types.ServiceConfig

func vizCommand(p *ProjectOptions) *cobra.Command {
	opts := vizOptions{
		ProjectOptions: p,
	}
	var indentationSize int
	var useSpaces bool

	cmd := &cobra.Command{
		Use:   "viz [OPTIONS]",
		Short: "EXPERIMENTAL - Generate a graphviz graph from your compose file",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			var err error
			opts.indentationStr, err = preferredIndentationStr(indentationSize, useSpaces)
			return err
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runViz(ctx, &opts)
		}),
	}

	cmd.Flags().BoolVar(&opts.includePorts, "ports", false, "Include service's exposed ports in output graph")
	cmd.Flags().BoolVar(&opts.includeNetworks, "networks", false, "Include service's attached networks in output graph")
	cmd.Flags().BoolVar(&opts.includeImageName, "image", false, "Include service's image name in output graph")
	cmd.Flags().IntVar(&indentationSize, "indentation-size", 1, "Number of tabs or spaces to use for indentation")
	cmd.Flags().BoolVar(&useSpaces, "spaces", false, "If given, space character ' ' will be used to indent,\notherwise tab character '\\t' will be used")
	return cmd
}

func runViz(_ context.Context, opts *vizOptions) error {
	_, _ = fmt.Fprintln(os.Stderr, "viz command is EXPERIMENTAL")
	project, err := opts.ToProject(nil)
	if err != nil {
		return err
	}

	// build graph
	graph := make(vizGraph)
	for i, serviceConfig := range project.Services {
		serviceConfigPtr := &project.Services[i]
		graph[serviceConfigPtr] = make([]*types.ServiceConfig, 0, len(serviceConfig.DependsOn))
		for dependencyName := range serviceConfig.DependsOn {
			// no error should be returned since dependencyName should exist
			dependency, _ := project.GetService(dependencyName)
			graph[serviceConfigPtr] = append(graph[serviceConfigPtr], &dependency)
		}
	}

	// build graphviz graph
	var graphBuilder strings.Builder
	graphBuilder.WriteString("digraph " + project.Name + " {\n")
	graphBuilder.WriteString(opts.indentationStr + "layout=dot;\n")
	addNodes(&graphBuilder, graph, opts)
	graphBuilder.WriteByte('\n')
	addEdges(&graphBuilder, graph, opts)
	graphBuilder.WriteString("}\n")

	fmt.Println(graphBuilder.String())

	return nil
}

// addNodes adds the corresponding graphviz representation of all the nodes in the given graph to the graphBuilder
// returns the same graphBuilder
func addNodes(graphBuilder *strings.Builder, graph vizGraph, opts *vizOptions) *strings.Builder {
	for serviceNode := range graph {
		// write:
		// "service name" [style="filled" label<<font point-size="15">service name</font>
		graphBuilder.WriteString(opts.indentationStr)
		writeQuoted(graphBuilder, serviceNode.Name)
		graphBuilder.WriteString(" [style=\"filled\" label=<<font point-size=\"15\">")
		graphBuilder.WriteString(serviceNode.Name)
		graphBuilder.WriteString("</font>")

		if opts.includeNetworks && len(serviceNode.Networks) > 0 {
			graphBuilder.WriteString("<font point-size=\"10\">")
			graphBuilder.WriteString("<br/><br/><b>Networks:</b>")
			for _, networkName := range serviceNode.NetworksByPriority() {
				graphBuilder.WriteString("<br/>")
				graphBuilder.WriteString(networkName)
			}
			graphBuilder.WriteString("</font>")
		}

		if opts.includePorts && len(serviceNode.Ports) > 0 {
			graphBuilder.WriteString("<font point-size=\"10\">")
			graphBuilder.WriteString("<br/><br/><b>Ports:</b>")
			for _, portConfig := range serviceNode.Ports {
				graphBuilder.WriteString("<br/>")
				if len(portConfig.HostIP) > 0 {
					graphBuilder.WriteString(portConfig.HostIP)
					graphBuilder.WriteByte(':')
				}
				graphBuilder.WriteString(portConfig.Published)
				graphBuilder.WriteByte(':')
				graphBuilder.WriteString(strconv.Itoa(int(portConfig.Target)))
				graphBuilder.WriteString(" (")
				graphBuilder.WriteString(portConfig.Protocol)
				graphBuilder.WriteString(", ")
				graphBuilder.WriteString(portConfig.Mode)
				graphBuilder.WriteString(")")
			}
			graphBuilder.WriteString("</font>")
		}

		if opts.includeImageName {
			graphBuilder.WriteString("<font point-size=\"10\">")
			graphBuilder.WriteString("<br/><br/><b>Image:</b><br/>")
			graphBuilder.WriteString(serviceNode.Image)
			graphBuilder.WriteString("</font>")
		}

		graphBuilder.WriteString(">];\n")
	}

	return graphBuilder
}

// addEdges adds the corresponding graphviz representation of all edges in the given graph to the graphBuilder
// returns the same graphBuilder
func addEdges(graphBuilder *strings.Builder, graph vizGraph, opts *vizOptions) *strings.Builder {
	for parent, children := range graph {
		for _, child := range children {
			graphBuilder.WriteString(opts.indentationStr)
			writeQuoted(graphBuilder, parent.Name)
			graphBuilder.WriteString(" -> ")
			writeQuoted(graphBuilder, child.Name)
			graphBuilder.WriteString(";\n")
		}
	}

	return graphBuilder
}

// writeQuoted writes "str" to builder
func writeQuoted(builder *strings.Builder, str string) {
	builder.WriteByte('"')
	builder.WriteString(str)
	builder.WriteByte('"')
}

// preferredIndentationStr returns a single string given the indentation preference
func preferredIndentationStr(size int, useSpace bool) (string, error) {
	if size < 0 {
		return "", fmt.Errorf("invalid indentation size: %d", size)
	}

	indentationStr := "\t"
	if useSpace {
		indentationStr = " "
	}
	return strings.Repeat(indentationStr, size), nil
}
