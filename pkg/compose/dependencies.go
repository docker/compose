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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
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

type graphTraversal struct {
	mu      sync.Mutex
	seen    map[string]struct{}
	ignored map[string]struct{}

	extremityNodesFn            func(*Graph) []*Vertex                        // leaves or roots
	adjacentNodesFn             func(*Vertex) []*Vertex                       // getParents or getChildren
	filterAdjacentByStatusFn    func(*Graph, string, ServiceStatus) []*Vertex // filterChildren or filterParents
	targetServiceStatus         ServiceStatus
	adjacentServiceStatusToSkip ServiceStatus

	visitorFn      func(context.Context, string) error
	maxConcurrency int
}

func upDirectionTraversal(visitorFn func(context.Context, string) error) *graphTraversal {
	return &graphTraversal{
		extremityNodesFn:            leaves,
		adjacentNodesFn:             getParents,
		filterAdjacentByStatusFn:    filterChildren,
		adjacentServiceStatusToSkip: ServiceStopped,
		targetServiceStatus:         ServiceStarted,
		visitorFn:                   visitorFn,
	}
}

func downDirectionTraversal(visitorFn func(context.Context, string) error) *graphTraversal {
	return &graphTraversal{
		extremityNodesFn:            roots,
		adjacentNodesFn:             getChildren,
		filterAdjacentByStatusFn:    filterParents,
		adjacentServiceStatusToSkip: ServiceStarted,
		targetServiceStatus:         ServiceStopped,
		visitorFn:                   visitorFn,
	}
}

// InDependencyOrder applies the function to the services of the project taking in account the dependency order
func InDependencyOrder(ctx context.Context, project *types.Project, fn func(context.Context, string) error, options ...func(*graphTraversal)) error {
	graph, err := NewGraph(project, ServiceStopped)
	if err != nil {
		return err
	}
	t := upDirectionTraversal(fn)
	for _, option := range options {
		option(t)
	}
	return t.visit(ctx, graph)
}

// InReverseDependencyOrder applies the function to the services of the project in reverse order of dependencies
func InReverseDependencyOrder(ctx context.Context, project *types.Project, fn func(context.Context, string) error, options ...func(*graphTraversal)) error {
	graph, err := NewGraph(project, ServiceStarted)
	if err != nil {
		return err
	}
	t := downDirectionTraversal(fn)
	for _, option := range options {
		option(t)
	}
	return t.visit(ctx, graph)
}

func WithRootNodesAndDown(nodes []string) func(*graphTraversal) {
	return func(t *graphTraversal) {
		if len(nodes) == 0 {
			return
		}
		originalFn := t.extremityNodesFn
		t.extremityNodesFn = func(graph *Graph) []*Vertex {
			var want []string
			for _, node := range nodes {
				vertex := graph.Vertices[node]
				want = append(want, vertex.Service)
				for _, v := range getAncestors(vertex) {
					want = append(want, v.Service)
				}
			}

			t.ignored = map[string]struct{}{}
			for k := range graph.Vertices {
				if !utils.Contains(want, k) {
					t.ignored[k] = struct{}{}
				}
			}

			return originalFn(graph)
		}
	}
}

func (t *graphTraversal) visit(ctx context.Context, g *Graph) error {
	expect := len(g.Vertices)
	if expect == 0 {
		return nil
	}

	eg, ctx := errgroup.WithContext(ctx)
	if t.maxConcurrency > 0 {
		eg.SetLimit(t.maxConcurrency + 1)
	}
	nodeCh := make(chan *Vertex, expect)
	defer close(nodeCh)
	// nodeCh need to allow n=expect writers while reader goroutine could have returner after ctx.Done
	eg.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case node := <-nodeCh:
				expect--
				if expect == 0 {
					return nil
				}
				t.run(ctx, g, eg, t.adjacentNodesFn(node), nodeCh)
			}
		}
	})

	nodes := t.extremityNodesFn(g)
	t.run(ctx, g, eg, nodes, nodeCh)

	return eg.Wait()
}

// Note: this could be `graph.walk` or whatever
func (t *graphTraversal) run(ctx context.Context, graph *Graph, eg *errgroup.Group, nodes []*Vertex, nodeCh chan *Vertex) {
	for _, node := range nodes {
		// Don't start this service yet if all of its children have
		// not been started yet.
		if len(t.filterAdjacentByStatusFn(graph, node.Key, t.adjacentServiceStatusToSkip)) != 0 {
			continue
		}

		node := node
		if !t.consume(node.Key) {
			// another worker already visited this node
			continue
		}

		eg.Go(func() error {
			var err error
			if _, ignore := t.ignored[node.Service]; !ignore {
				err = t.visitorFn(ctx, node.Service)
			}
			if err == nil {
				graph.UpdateStatus(node.Key, t.targetServiceStatus)
			}
			nodeCh <- node
			return err
		})
	}
}

func (t *graphTraversal) consume(nodeKey string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.seen == nil {
		t.seen = make(map[string]struct{})
	}
	if _, ok := t.seen[nodeKey]; ok {
		return false
	}
	t.seen[nodeKey] = struct{}{}
	return true
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

// GetParents returns a slice with the parent vertices of the Vertex
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

// getAncestors return all descendents for a vertex, might contain duplicates
func getAncestors(v *Vertex) []*Vertex {
	var descendents []*Vertex
	for _, parent := range v.GetParents() {
		descendents = append(descendents, parent)
		descendents = append(descendents, getAncestors(parent)...)
	}
	return descendents
}

// GetChildren returns a slice with the child vertices of the Vertex
func (v *Vertex) GetChildren() []*Vertex {
	var res []*Vertex
	for _, p := range v.Children {
		res = append(res, p)
	}
	return res
}

// NewGraph returns the dependency graph of the services
func NewGraph(project *types.Project, initialStatus ServiceStatus) (*Graph, error) {
	graph := &Graph{
		lock:     sync.RWMutex{},
		Vertices: map[string]*Vertex{},
	}

	for _, s := range project.Services {
		graph.AddVertex(s.Name, s.Name, initialStatus)
	}

	for index, s := range project.Services {
		for _, name := range s.GetDependencies() {
			err := graph.AddEdge(s.Name, name)
			if err != nil {
				if !s.DependsOn[name].Required {
					delete(s.DependsOn, name)
					project.Services[index] = s
					continue
				}
				if api.IsNotFoundError(err) {
					ds, err := project.GetDisabledService(name)
					if err == nil {
						return nil, fmt.Errorf("service %s is required by %s but is disabled. Can be enabled by profiles %s", name, s.Name, ds.Profiles)
					}
				}
				return nil, err
			}
		}
	}

	if b, err := graph.HasCycles(); b {
		return nil, err
	}

	return graph, nil
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

// AddEdge adds a relationship of dependency between vertices `source` and `destination`
func (g *Graph) AddEdge(source string, destination string) error {
	g.lock.Lock()
	defer g.lock.Unlock()

	sourceVertex := g.Vertices[source]
	destinationVertex := g.Vertices[destination]

	if sourceVertex == nil {
		return fmt.Errorf("could not find %s: %w", source, api.ErrNotFound)
	}
	if destinationVertex == nil {
		return fmt.Errorf("could not find %s: %w", destination, api.ErrNotFound)
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
