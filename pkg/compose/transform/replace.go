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

package transform

import (
	"fmt"

	"go.yaml.in/yaml/v4"
)

// ReplaceExtendsFile changes value for service.extends.file in input yaml stream, preserving formatting
func ReplaceExtendsFile(in []byte, serviceName string, value string) ([]byte, error) {
	var doc yaml.Node
	err := yaml.Unmarshal(in, &doc)
	if err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode {
		return nil, fmt.Errorf("expected document kind %v, got %v", yaml.DocumentNode, doc.Kind)
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected document root to be a mapping, got %v", root.Kind)
	}

	services, err := getMapping(root, "services")
	if err != nil {
		return nil, err
	}

	target, err := getMapping(services, serviceName)
	if err != nil {
		return nil, err
	}

	extends, err := getMapping(target, "extends")
	if err != nil {
		return nil, err
	}

	file, err := getMapping(extends, "file")
	if err != nil {
		return nil, err
	}

	// we've found target `file` yaml node. Let's replace value in stream at node position
	if isFlowStyle(root, services, target, extends, file) {
		return nil, fmt.Errorf("service %q: extends.file is in a flow-style mapping, which cannot be rewritten while preserving formatting", serviceName)
	}
	return replace(in, file.Line, file.Column, value)
}

// ReplaceEnvFile changes value for service.extends.env_file in input yaml stream, preserving formatting
func ReplaceEnvFile(in []byte, serviceName string, i int, value string) ([]byte, error) {
	var doc yaml.Node
	err := yaml.Unmarshal(in, &doc)
	if err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode {
		return nil, fmt.Errorf("expected document kind %v, got %v", yaml.DocumentNode, doc.Kind)
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected document root to be a mapping, got %v", root.Kind)
	}

	services, err := getMapping(root, "services")
	if err != nil {
		return nil, err
	}

	target, err := getMapping(services, serviceName)
	if err != nil {
		return nil, err
	}

	envFile, err := getMapping(target, "env_file")
	if err != nil {
		return nil, err
	}

	// env_file can be either a string, sequence of strings, or sequence of mappings with path attribute
	if envFile.Kind == yaml.SequenceNode {
		if i < 0 || i >= len(envFile.Content) {
			return nil, fmt.Errorf("service %q: env_file has %d entries, cannot rewrite entry %d", serviceName, len(envFile.Content), i)
		}
		if isFlowStyle(root, services, target, envFile) || isFlowStyle(envFile.Content[i]) {
			return nil, fmt.Errorf("service %q: env_file is in flow style, which cannot be rewritten while preserving formatting", serviceName)
		}
		envFile = envFile.Content[i]
		if envFile.Kind == yaml.MappingNode {
			envFile, err = getMapping(envFile, "path")
			if err != nil {
				return nil, err
			}
		}
		return replace(in, envFile.Line, envFile.Column, value)
	} else {
		if isFlowStyle(root, services, target, envFile) {
			return nil, fmt.Errorf("service %q: env_file is in flow style, which cannot be rewritten while preserving formatting", serviceName)
		}
		return replace(in, envFile.Line, envFile.Column, value)
	}
}

func getMapping(root *yaml.Node, key string) (*yaml.Node, error) {
	var node *yaml.Node
	l := len(root.Content)
	for i := 0; i < l; i += 2 {
		k := root.Content[i]
		if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
			return nil, fmt.Errorf("expected mapping key to be a string, got %v %v", root.Kind, k.Tag)
		}
		if k.Value == key {
			node = root.Content[i+1]
			return node, nil
		}
	}
	return nil, fmt.Errorf("key %v not found", key)
}

// replace changes yaml node value in stream at position, preserving content
func replace(in []byte, line int, column int, value string) ([]byte, error) {
	var out []byte
	l := 1
	pos := 0
	// the scan looks for the newline that precedes the target line; line 1
	// has none, so without the guard pos would run to the end of the stream
	// and the column offset below would slice out of range
	if line > 1 {
		for _, b := range in {
			if b == '\n' {
				l++
				if l == line {
					break
				}
			}
			pos++
		}
		// pos stopped at the newline before the target line, one byte before
		// the line start; columns are 1-based, so the value starts at
		// line-start + column - 1, which is pos + column
		pos += column
	} else {
		// line 1 starts at offset 0, so the 1-based column maps to column - 1
		pos = column - 1
	}
	if pos > len(in) {
		return nil, fmt.Errorf("cannot locate line %d column %d in stream", line, column)
	}
	out = append(out, in[0:pos]...)
	out = append(out, []byte(value)...)
	for ; pos < len(in); pos++ {
		if in[pos] == '\n' {
			break
		}
	}
	out = append(out, in[pos:]...)
	return out, nil
}

// isFlowStyle reports whether any node on the path to a rewritten value is
// in flow style. replace rewrites from the value's start to the end of its
// line — anything after the value on that line, a trailing comment
// included, is dropped — so it is only safe when the value is the sole
// content of its line: flow style puts closing braces, commas and sibling
// keys after the value on the same line, and swallowing them would corrupt
// the stream.
func isFlowStyle(nodes ...*yaml.Node) bool {
	for _, n := range nodes {
		if n != nil && n.Style&yaml.FlowStyle != 0 {
			return true
		}
	}
	return false
}
