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
	"fmt"
	"os"

	"go.yaml.in/yaml/v4"
)

// declaredBlock captures what was parsed out of a `variables:` mapping
// in a Compose file (or out of a standalone variables file). Only
// flat scalar entries are accepted; external files are loaded via
// the sibling `variables_file:` key, not from inside the block.
type declaredBlock struct {
	// Inline holds key/value entries declared directly in the block,
	// in declaration order.
	Inline []rawEntry
}

// rawEntry is a name/value pair pre-coercion.
type rawEntry struct {
	Name  string
	Value any
}

// parseDeclaredOrdered walks a yaml.Node directly to keep declaration
// order intact (essential for cross-variable resolution).
func parseDeclaredOrdered(node *yaml.Node) (*declaredBlock, error) {
	if node == nil {
		return &declaredBlock{}, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping for `variables:`, got kind %d", node.Kind)
	}
	out := &declaredBlock{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("variable name must be a scalar, got kind %d", k.Kind)
		}
		var raw any
		if err := v.Decode(&raw); err != nil {
			return nil, fmt.Errorf("variable %q: %w", k.Value, err)
		}
		out.Inline = append(out.Inline, rawEntry{Name: k.Value, Value: raw})
	}
	return out, nil
}

// nodeToStringList accepts scalar string or sequence of scalar strings.
func nodeToStringList(node *yaml.Node) ([]string, error) {
	if node == nil {
		return nil, nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		return []string{node.Value}, nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, c := range node.Content {
			if c.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("expected scalar in list, got kind %d", c.Kind)
			}
			out = append(out, c.Value)
		}
		return out, nil
	}
	return nil, fmt.Errorf("expected scalar or sequence, got kind %d", node.Kind)
}

// LoadVarsFile reads an external variables YAML file. The file must
// have a top-level `variables:` mapping (flat scalars only). The
// returned slice preserves declaration order. Coercion happens here.
func LoadVarsFile(path string, source Source) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("parse %s: empty document", path)
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("parse %s: expected mapping at top level", path)
	}
	var varsNode *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Kind == yaml.ScalarNode && root.Content[i].Value == "variables" {
			varsNode = root.Content[i+1]
			break
		}
	}
	if varsNode == nil {
		return nil, fmt.Errorf("parse %s: top-level `variables:` key missing", path)
	}
	block, err := parseDeclaredOrdered(varsNode)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out := make([]Entry, 0, len(block.Inline))
	for _, e := range block.Inline {
		v, err := Coerce(e.Name, e.Value)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		out = append(out, Entry{Name: e.Name, Value: v, Source: source, From: path})
	}
	return out, nil
}

// ParseCLIVars converts repeated `KEY=VALUE` arguments into entries.
// Empty value is allowed; missing `=` is an error.
func ParseCLIVars(args []string) ([]Entry, error) {
	out := make([]Entry, 0, len(args))
	for _, a := range args {
		k, v, ok := splitKV(a)
		if !ok {
			return nil, fmt.Errorf("--var expects KEY=VALUE, got %q", a)
		}
		out = append(out, Entry{Name: k, Value: v, Source: SourceCLIVar, From: "--var " + a})
	}
	return out, nil
}

func splitKV(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}
