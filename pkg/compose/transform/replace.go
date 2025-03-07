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

	"gopkg.in/yaml.v3"
)

// ReplaceExtendsFile changes value for service.extends.file in input yaml stream, preserving formatting
func ReplaceExtendsFile(in []byte, service string, value string) ([]byte, error) {
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

	target, err := getMapping(services, service)
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
	return replace(in, file.Line, file.Column, value), nil
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
func replace(in []byte, line int, column int, value string) []byte {
	var out []byte
	l := 1
	pos := 0
	for _, b := range in {
		if b == '\n' {
			l++
			if l == line {
				break
			}
		}
		pos++
	}
	pos += column
	out = append(out, in[0:pos]...)
	out = append(out, []byte(value)...)
	for ; pos < len(in); pos++ {
		if in[pos] == '\n' {
			break
		}
	}
	out = append(out, in[pos:]...)
	return out
}
