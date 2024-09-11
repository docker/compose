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
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
)

// maps a service with the services it depends on
type vizGraph map[*types.ServiceConfig][]*types.ServiceConfig

func (s *composeService) Viz(_ context.Context, project *types.Project, opts api.VizOptions) (string, error) {
	graph := make(vizGraph)
	for _, service := range project.Services {
		service := service
		graph[&service] = make([]*types.ServiceConfig, 0, len(service.DependsOn))
		for dependencyName := range service.DependsOn {
			// no error should be returned since dependencyName should exist
			dependency, _ := project.GetService(dependencyName)
			graph[&service] = append(graph[&service], &dependency)
		}
	}

	// build graphviz graph
	var graphBuilder strings.Builder

	// graph name
	graphBuilder.WriteString("digraph ")
	writeQuoted(&graphBuilder, project.Name)
	graphBuilder.WriteString(" {\n")

	// graph layout
	// dot is the perfect layout for this use case since graph is directed and hierarchical
	graphBuilder.WriteString(opts.Indentation + "layout=dot;\n")

	addNodes(&graphBuilder, graph, project.Name, &opts)
	graphBuilder.WriteByte('\n')

	addEdges(&graphBuilder, graph, &opts)
	graphBuilder.WriteString("}\n")

	return graphBuilder.String(), nil
}

// addNodes adds the corresponding graphviz representation of all the nodes in the given graph to the graphBuilder
// returns the same graphBuilder
func addNodes(graphBuilder *strings.Builder, graph vizGraph, projectName string, opts *api.VizOptions) *strings.Builder {
	for serviceNode := range graph {
		// write:
		// "service name" [style="filled" label<<font point-size="15">service name</font>
		graphBuilder.WriteString(opts.Indentation)
		writeQuoted(graphBuilder, serviceNode.Name)
		graphBuilder.WriteString(" [style=\"filled\" label=<<font point-size=\"15\">")
		graphBuilder.WriteString(serviceNode.Name)
		graphBuilder.WriteString("</font>")

		if opts.IncludeNetworks && len(serviceNode.Networks) > 0 {
			graphBuilder.WriteString("<font point-size=\"10\">")
			graphBuilder.WriteString("<br/><br/><b>Networks:</b>")
			for _, networkName := range serviceNode.NetworksByPriority() {
				graphBuilder.WriteString("<br/>")
				graphBuilder.WriteString(networkName)
			}
			graphBuilder.WriteString("</font>")
		}

		if opts.IncludePorts && len(serviceNode.Ports) > 0 {
			graphBuilder.WriteString("<font point-size=\"10\">")
			graphBuilder.WriteString("<br/><br/><b>Ports:</b>")
			for _, portConfig := range serviceNode.Ports {
				graphBuilder.WriteString("<br/>")
				if portConfig.HostIP != "" {
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

		if opts.IncludeImageName {
			graphBuilder.WriteString("<font point-size=\"10\">")
			graphBuilder.WriteString("<br/><br/><b>Image:</b><br/>")
			graphBuilder.WriteString(api.GetImageNameOrDefault(*serviceNode, projectName))
			graphBuilder.WriteString("</font>")
		}

		graphBuilder.WriteString(">];\n")
	}

	return graphBuilder
}

// addEdges adds the corresponding graphviz representation of all edges in the given graph to the graphBuilder
// returns the same graphBuilder
func addEdges(graphBuilder *strings.Builder, graph vizGraph, opts *api.VizOptions) *strings.Builder {
	for parent, children := range graph {
		for _, child := range children {
			graphBuilder.WriteString(opts.Indentation)
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
