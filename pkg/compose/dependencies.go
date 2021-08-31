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

package compose

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/types"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/utils"
)

// ServiceStatus indicates the status of a service
type ServiceStatus int

// Services status flags
const (
	ServiceStopped ServiceStatus = iota
	ServiceStarted
)

type graphTraversalConfig struct {
	extremityNodesFn            func(*Graph) []*Vertex                        // leaves or roots
	adjacentNodesFn             func(*Vertex) []*Vertex                       // getParents or getChildren
	filterAdjacentByStatusFn    func(*Graph, string, ServiceStatus) []*Vertex // filterChildren or filterParents
	targetServiceStatus         ServiceStatus
	adjacentServiceStatusToSkip ServiceStatus
}

var (
	upDirectionTraversalConfig = graphTraversalConfig{
		extremityNodesFn:            leaves,
		adjacentNodesFn:             getParents,
		filterAdjacentByStatusFn:    filterChildren,
		adjacentServiceStatusToSkip: ServiceStopped,
		targetServiceStatus:         ServiceStarted,
	}
	downDirectionTraversalConfig = graphTraversalConfig{
		extremityNodesFn:            roots,
		adjacentNodesFn:             getChildren,
		filterAdjacentByStatusFn:    filterParents,
		adjacentServiceStatusToSkip: ServiceStarted,
		targetServiceStatus:         ServiceStopped,
	}
)

// InDependencyOrder applies the function to the services of the project taking in account the dependency order
func InDependencyOrder(ctx context.Context, project *types.Project, fn func(context.Context, string) error) error {
	return visit(ctx, project, upDirectionTraversalConfig, fn, ServiceStopped)
}

// InReverseDependencyOrder applies the function to the services of the project in reverse order of dependencies
func InReverseDependencyOrder(ctx context.Context, project *types.Project, fn func(context.Context, string) error) error {
	return visit(ctx, project, downDirectionTraversalConfig, fn, ServiceStarted)
}

func visit(ctx context.Context, project *types.Project, traversalConfig graphTraversalConfig, fn func(context.Context, string) error, initialStatus ServiceStatus) error {
	g := NewGraph(project.Services, initialStatus)
	if b, err := g.HasCycles(); b {
		return err
	}

	nodes := traversalConfig.extremityNodesFn(g)

	eg, _ := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return run(ctx, g, eg, nodes, traversalConfig, fn)
	})

	return eg.Wait()
}

// Note: this could be `graph.walk` or whatever
func run(ctx context.Context, graph *Graph, eg *errgroup.Group, nodes []*Vertex, traversalConfig graphTraversalConfig, fn func(context.Context, string) error) error {
	for _, node := range nodes {
		// Don't start this service yet if all of its children have
		// not been started yet.
		if len(traversalConfig.filterAdjacentByStatusFn(graph, node.Service, traversalConfig.adjacentServiceStatusToSkip)) != 0 {
			continue
		}

		node := node
		eg.Go(func() error {
			err := fn(ctx, node.Service)
			if err != nil {
				return err
			}

			graph.UpdateStatus(node.Service, traversalConfig.targetServiceStatus)

			return run(ctx, graph, eg, traversalConfig.adjacentNodesFn(node), traversalConfig, fn)
		})
	}

	return nil
}

// Graph represents project as service dependencies
type Graph struct {
	Vertices map[string]*Vertex
	lock     sync.RWMutex
}

// Vertex represents a service in the dependencies structure
type Vertex struct {
	Key      string
	Service  string
	Status   ServiceStatus
	Children map[string]*Vertex
	Parents  map[string]*Vertex
}

func getParents(v *Vertex) []*Vertex {
	return v.GetParents()
}

// GetParents returns a slice with the parent vertexes of the a Vertex
func (v *Vertex) GetParents() []*Vertex {
	var res []*Vertex
	for _, p := range v.Parents {
		res = append(res, p)
	}
	return res
}

func getChildren(v *Vertex) []*Vertex {
	return v.GetChildren()
}

// GetChildren returns a slice with the child vertexes of the a Vertex
func (v *Vertex) GetChildren() []*Vertex {
	var res []*Vertex
	for _, p := range v.Children {
		res = append(res, p)
	}
	return res
}

// NewGraph returns the dependency graph of the services
func NewGraph(services types.Services, initialStatus ServiceStatus) *Graph {
	graph := &Graph{
		lock:     sync.RWMutex{},
		Vertices: map[string]*Vertex{},
	}

	for _, s := range services {
		graph.AddVertex(s.Name, s.Name, initialStatus)
	}

	for _, s := range services {
		for _, name := range s.GetDependencies() {
			_ = graph.AddEdge(s.Name, name)
		}
	}

	return graph
}

// NewVertex is the constructor function for the Vertex
func NewVertex(key string, service string, initialStatus ServiceStatus) *Vertex {
	return &Vertex{
		Key:      key,
		Service:  service,
		Status:   initialStatus,
		Parents:  map[string]*Vertex{},
		Children: map[string]*Vertex{},
	}
}

// AddVertex adds a vertex to the Graph
func (g *Graph) AddVertex(key string, service string, initialStatus ServiceStatus) {
	g.lock.Lock()
	defer g.lock.Unlock()

	v := NewVertex(key, service, initialStatus)
	g.Vertices[key] = v
}

// AddEdge adds a relationship of dependency between vertexes `source` and `destination`
func (g *Graph) AddEdge(source string, destination string) error {
	g.lock.Lock()
	defer g.lock.Unlock()

	sourceVertex := g.Vertices[source]
	destinationVertex := g.Vertices[destination]

	if sourceVertex == nil {
		return fmt.Errorf("could not find %s", source)
	}
	if destinationVertex == nil {
		return fmt.Errorf("could not find %s", destination)
	}

	// If they are already connected
	if _, ok := sourceVertex.Children[destination]; ok {
		return nil
	}

	sourceVertex.Children[destination] = destinationVertex
	destinationVertex.Parents[source] = sourceVertex

	return nil
}

func leaves(g *Graph) []*Vertex {
	return g.Leaves()
}

// Leaves returns the slice of leaves of the graph
func (g *Graph) Leaves() []*Vertex {
	g.lock.Lock()
	defer g.lock.Unlock()

	var res []*Vertex
	for _, v := range g.Vertices {
		if len(v.Children) == 0 {
			res = append(res, v)
		}
	}

	return res
}

func roots(g *Graph) []*Vertex {
	return g.Roots()
}

// Roots returns the slice of "Roots" of the graph
func (g *Graph) Roots() []*Vertex {
	g.lock.Lock()
	defer g.lock.Unlock()

	var res []*Vertex
	for _, v := range g.Vertices {
		if len(v.Parents) == 0 {
			res = append(res, v)
		}
	}
	return res
}

// UpdateStatus updates the status of a certain vertex
func (g *Graph) UpdateStatus(key string, status ServiceStatus) {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.Vertices[key].Status = status
}

func filterChildren(g *Graph, k string, s ServiceStatus) []*Vertex {
	return g.FilterChildren(k, s)
}

// FilterChildren returns children of a certain vertex that are in a certain status
func (g *Graph) FilterChildren(key string, status ServiceStatus) []*Vertex {
	g.lock.Lock()
	defer g.lock.Unlock()

	var res []*Vertex
	vertex := g.Vertices[key]

	for _, child := range vertex.Children {
		if child.Status == status {
			res = append(res, child)
		}
	}

	return res
}

func filterParents(g *Graph, k string, s ServiceStatus) []*Vertex {
	return g.FilterParents(k, s)
}

// FilterParents returns the parents of a certain vertex that are in a certain status
func (g *Graph) FilterParents(key string, status ServiceStatus) []*Vertex {
	g.lock.Lock()
	defer g.lock.Unlock()

	var res []*Vertex
	vertex := g.Vertices[key]

	for _, parent := range vertex.Parents {
		if parent.Status == status {
			res = append(res, parent)
		}
	}

	return res
}

// HasCycles detects cycles in the graph
func (g *Graph) HasCycles() (bool, error) {
	discovered := []string{}
	finished := []string{}

	for _, vertex := range g.Vertices {
		path := []string{
			vertex.Key,
		}
		if !utils.StringContains(discovered, vertex.Key) && !utils.StringContains(finished, vertex.Key) {
			var err error
			discovered, finished, err = g.visit(vertex.Key, path, discovered, finished)

			if err != nil {
				return true, err
			}
		}
	}

	return false, nil
}

func (g *Graph) visit(key string, path []string, discovered []string, finished []string) ([]string, []string, error) {
	discovered = append(discovered, key)

	for _, v := range g.Vertices[key].Children {
		path := append(path, v.Key)
		if utils.StringContains(discovered, v.Key) {
			return nil, nil, fmt.Errorf("cycle found: %s", strings.Join(path, " -> "))
		}

		if !utils.StringContains(finished, v.Key) {
			if _, _, err := g.visit(v.Key, path, discovered, finished); err != nil {
				return nil, nil, err
			}
		}
	}

	discovered = remove(discovered, key)
	finished = append(finished, key)
	return discovered, finished, nil
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
