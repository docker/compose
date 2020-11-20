// +build local

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

package local

import (
	"context"

	"github.com/compose-spec/compose-go/types"
	"golang.org/x/sync/errgroup"
)

func inDependencyOrder(ctx context.Context, project *types.Project, fn func(context.Context, types.ServiceConfig) error) error {
	graph := buildDependencyGraph(project.Services)

	eg, ctx := errgroup.WithContext(ctx)
	results := make(chan string)
	errors := make(chan error)
	scheduled := map[string]bool{}
	for len(graph) > 0 {
		for _, n := range graph.independents() {
			service := n.service
			if scheduled[service.Name] {
				continue
			}
			eg.Go(func() error {
				err := fn(ctx, service)
				if err != nil {
					errors <- err
					return err
				}
				results <- service.Name
				return nil
			})
			scheduled[service.Name] = true
		}
		select {
		case result := <-results:
			graph.resolved(result)
		case err := <-errors:
			return err
		}
	}
	return eg.Wait()
}

type dependencyGraph map[string]node

type node struct {
	service      types.ServiceConfig
	dependencies []string
	dependent    []string
}

func (graph dependencyGraph) independents() []node {
	var nodes []node
	for _, node := range graph {
		if len(node.dependencies) == 0 {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func (graph dependencyGraph) resolved(result string) {
	for _, parent := range graph[result].dependent {
		node := graph[parent]
		node.dependencies = remove(node.dependencies, result)
		graph[parent] = node
	}
	delete(graph, result)
}

func buildDependencyGraph(services types.Services) dependencyGraph {
	graph := dependencyGraph{}
	for _, s := range services {
		graph[s.Name] = node{
			service: s,
		}
	}

	for _, s := range services {
		node := graph[s.Name]
		for _, name := range s.GetDependencies() {
			dependency := graph[name]
			node.dependencies = append(node.dependencies, name)
			dependency.dependent = append(dependency.dependent, s.Name)
			graph[name] = dependency
		}
		graph[s.Name] = node
	}
	return graph
}

func remove(slice []string, item string) []string {
	var s []string
	for _, i := range slice {
		if i != item {
			s = append(s, i)
		}
	}
	return s
}
