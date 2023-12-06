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

type visitorFn func(context.Context, string, types.ServiceConfig) error

type graphTraversal struct {
	mu      sync.Mutex
	seen    map[string]struct{}
	ignored map[string]struct{}

	extremityNodesFn func(*Graph) []*Vertex         // leaves or roots
	adjacentNodesFn  func(*Vertex) []*Vertex        // getParents or getChildren
	filterAdjacentFn func(*Graph, string) []*Vertex // filterChildren or filterParents

	visitorFn      visitorFn
	maxConcurrency int
}

func upDirectionTraversal(visitorFn visitorFn) *graphTraversal {
	return &graphTraversal{
		extremityNodesFn: leaves,
		adjacentNodesFn:  getParents,
		filterAdjacentFn: filterChildren,
		visitorFn:        visitorFn,
	}
}

func downDirectionTraversal(visitorFn visitorFn) *graphTraversal {
	return &graphTraversal{
		extremityNodesFn: roots,
		adjacentNodesFn:  getChildren,
		filterAdjacentFn: filterParents,
		visitorFn:        visitorFn,
	}
}

// InDependencyOrder applies the function to the services of the project taking in account the dependency order
func InDependencyOrder(ctx context.Context, project *types.Project, fn visitorFn, options ...func(*graphTraversal)) error {
	graph, err := NewGraph(project)
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
func InReverseDependencyOrder(ctx context.Context, project *types.Project, fn visitorFn, options ...func(*graphTraversal)) error {
	graph, err := NewGraph(project)
	if err != nil {
		return err
	}
	t := downDirectionTraversal(fn)
	for _, option := range options {
		option(t)
	}
	return t.visit(ctx, graph)
}

// WithRootNodesAndDown creates a graphTraversal to start from nodes and walk down by dependencies
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
				want = append(want, vertex.Service.Name)
				for _, v := range getAncestors(vertex) {
					want = append(want, v.Service.Name)
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
		if len(t.filterAdjacentFn(graph, node.Key)) != 0 {
			continue
		}

		node := node
		if !t.consume(node.Key) {
			// another worker already visited this node
			continue
		}

		eg.Go(func() error {
			var err error
			if _, ignore := t.ignored[node.Key]; !ignore {
				err = t.visitorFn(ctx, node.Key, *node.Service)
			}
			if err == nil {
				graph.UpdateStatus(node.Key)
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
	Service  *types.ServiceConfig
	Visited  bool
	Children map[string]*Vertex
	Parents  map[string]*Vertex
}

func getParents(v *Vertex) []*Vertex {
	var res []*Vertex
	for _, p := range v.Parents {
		res = append(res, p)
	}
	return res
}

// getAncestors return all descendents for a vertex, might contain duplicates
func getAncestors(v *Vertex) []*Vertex {
	var descendents []*Vertex
	for _, parent := range getParents(v) {
		descendents = append(descendents, parent)
		descendents = append(descendents, getAncestors(parent)...)
	}
	return descendents
}

func getChildren(v *Vertex) []*Vertex {
	var res []*Vertex
	for _, p := range v.Children {
		res = append(res, p)
	}
	return res
}

// NewGraph returns the dependency graph of the services
func NewGraph(project *types.Project) (*Graph, error) {
	graph := &Graph{
		lock:     sync.RWMutex{},
		Vertices: map[string]*Vertex{},
	}

	for name, s := range project.Services {
		graph.addVertex(name, s)
	}

	for index, s := range project.Services {
		for _, name := range s.GetDependencies() {
			err := graph.addEdge(s.Name, name)
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

// newVertex is the constructor function for the Vertex
func newVertex(key string, service *types.ServiceConfig) *Vertex {
	return &Vertex{
		Key:      key,
		Service:  service,
		Parents:  map[string]*Vertex{},
		Children: map[string]*Vertex{},
	}
}

// addVertex adds a vertex to the Graph
func (g *Graph) addVertex(key string, service types.ServiceConfig) {
	g.lock.Lock()
	defer g.lock.Unlock()

	v := newVertex(key, &service)
	g.Vertices[key] = v
}

// addEdge adds a relationship of dependency between vertices `source` and `destination`
func (g *Graph) addEdge(source string, destination string) error {
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
func (g *Graph) UpdateStatus(key string) {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.Vertices[key].Visited = true
}

func filterChildren(g *Graph, key string) []*Vertex {
	g.lock.Lock()
	defer g.lock.Unlock()

	var res []*Vertex
	vertex := g.Vertices[key]

	for _, child := range vertex.Children {
		if !child.Visited {
			res = append(res, child)
		}
	}

	return res
}

func filterParents(g *Graph, key string) []*Vertex {
	g.lock.Lock()
	defer g.lock.Unlock()

	var res []*Vertex
	vertex := g.Vertices[key]

	for _, parent := range vertex.Parents {
		if !parent.Visited {
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
