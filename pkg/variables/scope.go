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

// Package variables implements the Compose-time `variables:` extension.
// It preprocesses Compose YAML files: extracts variable declarations,
// resolves cross-references, performs interpolation, strips extension
// keys, and emits cleaned YAML for compose-go to consume.
package variables

import (
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/v2/template"
)

// Source identifies where a variable's value originated.
type Source string

const (
	SourceShell            Source = "shell"
	SourceCLIVar           Source = "cli-var"
	SourceCLIVarFile       Source = "cli-var-file"
	SourceIncludeInline    Source = "include-inline"
	SourceIncludeFile      Source = "include-file"
	SourceRootInline       Source = "root-inline"
	SourceRootFile         Source = "root-file"
	SourceIncludedTopLevel Source = "included-top-level"
)

// Priority orders Source from highest (lowest int) to lowest (highest int).
// Used to pick the winner when the same name is declared in multiple
// sources.
func priority(s Source) int {
	switch s {
	case SourceShell:
		return 0
	case SourceCLIVar:
		return 1
	case SourceCLIVarFile:
		return 2
	case SourceIncludeInline:
		return 3
	case SourceIncludeFile:
		return 4
	case SourceRootInline:
		return 5
	case SourceRootFile:
		return 6
	case SourceIncludedTopLevel:
		return 7
	}
	return 99
}

// Entry is a single declared variable with its raw (un-substituted)
// value and provenance.
type Entry struct {
	Name   string
	Value  string
	Source Source
	From   string // file path or CLI arg
}

// Scope groups all variable declarations visible while interpolating a
// single Compose file. Multiple sources may declare the same name; the
// one with the highest precedence (lowest priority value) wins.
type Scope struct {
	// raw holds the winning (un-resolved) Entry per variable name.
	raw map[string]Entry
	// order is declaration order for stable iteration.
	order []string
	// all keeps every declaration for debug output.
	all []Entry
	// shell looks up shell environment values.
	shell func(string) (string, bool)
}

// NewScope builds an empty scope. shell may be nil (treated as miss).
func NewScope(shell func(string) (string, bool)) *Scope {
	if shell == nil {
		shell = func(string) (string, bool) { return "", false }
	}
	return &Scope{
		raw:   map[string]Entry{},
		shell: shell,
	}
}

// Add records an Entry. If a higher-priority Source already wrote
// this name, the new entry is dropped (but still kept in the debug
// log). Within the same Source, the FIRST writer wins (callers feed
// per-source entries in declaration order).
func (s *Scope) Add(e Entry) {
	s.all = append(s.all, e)
	cur, present := s.raw[e.Name]
	if !present {
		s.raw[e.Name] = e
		s.order = append(s.order, e.Name)
		return
	}
	// Replace only if new entry's source has higher priority (lower number).
	if priority(e.Source) < priority(cur.Source) {
		s.raw[e.Name] = e
	}
}

// AddAll appends a slice of entries.
func (s *Scope) AddAll(entries []Entry) {
	for _, e := range entries {
		s.Add(e)
	}
}

// All returns every declaration that was added, in input order
// (regardless of which one is "winning"). Used for debug output.
func (s *Scope) All() []Entry {
	out := make([]Entry, len(s.all))
	copy(out, s.all)
	return out
}

// Winners returns each name's winning entry, in declaration order.
func (s *Scope) Winners() []Entry {
	out := make([]Entry, 0, len(s.order))
	for _, n := range s.order {
		out = append(out, s.raw[n])
	}
	return out
}

// Resolve substitutes cross-variable references and returns a mapping
// of name → final value. Shell environment values override declared
// values. Cycles produce an error.
func (s *Scope) Resolve() (map[string]string, error) {
	out := map[string]string{}
	resolving := []string{}
	var resolve func(name string) (string, bool, error)
	resolve = func(name string) (string, bool, error) {
		if v, ok := s.shell(name); ok {
			return v, true, nil
		}
		e, ok := s.raw[name]
		if !ok {
			return "", false, nil
		}
		if v, done := out[name]; done {
			return v, true, nil
		}
		for _, r := range resolving {
			if r == name {
				chain := append(append([]string{}, resolving...), name)
				return "", false, fmt.Errorf("cyclic variable reference: %s", strings.Join(chain, " -> "))
			}
		}
		resolving = append(resolving, name)
		defer func() { resolving = resolving[:len(resolving)-1] }()

		var inner error
		substituted, err := template.Substitute(e.Value, func(n string) (string, bool) {
			if inner != nil {
				return "", false
			}
			v, ok, rerr := resolve(n)
			if rerr != nil {
				inner = rerr
				return "", false
			}
			return v, ok
		})
		if inner != nil {
			return "", false, inner
		}
		if err != nil {
			return "", false, fmt.Errorf("variable %q: %w", name, err)
		}
		out[name] = substituted
		return substituted, true, nil
	}
	for _, n := range s.order {
		if _, _, err := resolve(n); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Mapping builds the template.Mapping used to substitute Compose
// body fields. Shell wins over declared values; undeclared+unset
// names are reported as missing (caller decides warn/empty).
func (s *Scope) Mapping(resolved map[string]string) template.Mapping {
	return func(name string) (string, bool) {
		if v, ok := s.shell(name); ok {
			return v, true
		}
		if v, ok := resolved[name]; ok {
			return v, true
		}
		return "", false
	}
}

// Inherit returns a child Scope that has all of parent's declarations
// already merged in (lower priority than child's own additions). The
// child does NOT mutate the parent.
func (s *Scope) Inherit() *Scope {
	c := NewScope(s.shell)
	c.all = append(c.all, s.all...)
	for n, e := range s.raw {
		c.raw[n] = e
	}
	c.order = append(c.order, s.order...)
	return c
}
