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

package variables

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestScopeFirstWriterWinsWithinSource(t *testing.T) {
	s := NewScope(nil)
	s.Add(Entry{Name: "FOO", Value: "first", Source: SourceRootInline})
	s.Add(Entry{Name: "FOO", Value: "second", Source: SourceRootInline})

	resolved, err := s.Resolve()
	assert.NilError(t, err)
	assert.Equal(t, resolved["FOO"], "first")
}

func TestScopeHigherSourceOverridesLower(t *testing.T) {
	s := NewScope(nil)
	s.Add(Entry{Name: "FOO", Value: "from-root-file", Source: SourceRootFile})
	s.Add(Entry{Name: "FOO", Value: "from-cli", Source: SourceCLIVar})

	resolved, err := s.Resolve()
	assert.NilError(t, err)
	assert.Equal(t, resolved["FOO"], "from-cli")
}

func TestScopeShellWinsOverDeclared(t *testing.T) {
	shell := func(name string) (string, bool) {
		if name == "FOO" {
			return "from-shell", true
		}
		return "", false
	}
	s := NewScope(shell)
	s.Add(Entry{Name: "FOO", Value: "from-yaml", Source: SourceRootInline})

	resolved, _ := s.Resolve()
	mapping := s.Mapping(resolved)
	v, ok := mapping("FOO")
	assert.Assert(t, ok)
	assert.Equal(t, v, "from-shell")
}

func TestScopeCrossRefForward(t *testing.T) {
	s := NewScope(nil)
	s.Add(Entry{Name: "BASE", Value: "acme", Source: SourceRootInline})
	s.Add(Entry{Name: "IMAGE", Value: "${BASE}/api", Source: SourceRootInline})
	s.Add(Entry{Name: "TAG", Value: "1.4.2", Source: SourceRootInline})
	s.Add(Entry{Name: "FULL", Value: "${IMAGE}:${TAG}", Source: SourceRootInline})

	resolved, err := s.Resolve()
	assert.NilError(t, err)
	assert.Equal(t, resolved["FULL"], "acme/api:1.4.2")
}

func TestScopeCrossRefCycle(t *testing.T) {
	s := NewScope(nil)
	s.Add(Entry{Name: "A", Value: "${B}", Source: SourceRootInline})
	s.Add(Entry{Name: "B", Value: "${A}", Source: SourceRootInline})

	_, err := s.Resolve()
	assert.ErrorContains(t, err, "cyclic")
}

func TestScopeMissingVariableLeavesEmpty(t *testing.T) {
	s := NewScope(nil)
	s.Add(Entry{Name: "FOO", Value: "${UNDEFINED}", Source: SourceRootInline})

	resolved, err := s.Resolve()
	assert.NilError(t, err)
	assert.Equal(t, resolved["FOO"], "")
}

func TestScopeShellFillsCrossRef(t *testing.T) {
	shell := func(name string) (string, bool) {
		if name == "BASE" {
			return "shellbase", true
		}
		return "", false
	}
	s := NewScope(shell)
	s.Add(Entry{Name: "FULL", Value: "${BASE}/x", Source: SourceRootInline})

	resolved, err := s.Resolve()
	assert.NilError(t, err)
	assert.Equal(t, resolved["FULL"], "shellbase/x")
}

func TestScopeInheritDoesNotMutateParent(t *testing.T) {
	parent := NewScope(nil)
	parent.Add(Entry{Name: "FOO", Value: "parent", Source: SourceRootInline})

	child := parent.Inherit()
	child.Add(Entry{Name: "FOO", Value: "child", Source: SourceIncludeInline})

	pr, _ := parent.Resolve()
	cr, _ := child.Resolve()

	assert.Equal(t, pr["FOO"], "parent")
	assert.Equal(t, cr["FOO"], "child")
}

func TestScopeWinnersListedInDeclarationOrder(t *testing.T) {
	s := NewScope(nil)
	s.Add(Entry{Name: "B", Value: "2", Source: SourceRootInline})
	s.Add(Entry{Name: "A", Value: "1", Source: SourceRootInline})
	s.Add(Entry{Name: "C", Value: "3", Source: SourceRootInline})

	winners := s.Winners()
	got := make([]string, len(winners))
	for i, w := range winners {
		got[i] = w.Name
	}
	assert.Equal(t, strings.Join(got, ","), "B,A,C")
}
