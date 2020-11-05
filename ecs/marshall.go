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

package ecs

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/sanathkr/go-yaml"
)

func marshall(template *cloudformation.Template, format string) ([]byte, error) {
	var (
		source    func() ([]byte, error)
		marshal   func(in interface{}) ([]byte, error)
		unmarshal func(in []byte, out interface{}) error
	)
	switch format {
	case "yaml":
		source = template.YAML
		marshal = yaml.Marshal
		unmarshal = yaml.Unmarshal
	case "json":
		source = template.JSON
		marshal = func(in interface{}) ([]byte, error) {
			return json.MarshalIndent(in, "", "  ")
		}
		unmarshal = json.Unmarshal
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}

	raw, err := source()
	if err != nil {
		return nil, err
	}

	var unmarshalled interface{}
	if err := unmarshal(raw, &unmarshalled); err != nil {
		return nil, fmt.Errorf("invalid JSON: %s", err)
	}

	if input, ok := unmarshalled.(map[interface{}]interface{}); ok {
		if resources, ok := input["Resources"]; ok {
			for _, uresource := range resources.(map[interface{}]interface{}) {
				if resource, ok := uresource.(map[interface{}]interface{}); ok {
					if resource["Type"] == "AWS::ECS::TaskDefinition" {
						properties := resource["Properties"].(map[interface{}]interface{})
						for _, def := range properties["ContainerDefinitions"].([]interface{}) {
							containerDefinition := def.(map[interface{}]interface{})
							if strings.HasSuffix(containerDefinition["Name"].(string), "_InitContainer") {
								containerDefinition["Essential"] = false
							}
						}
					}
				}
			}
		}
	}

	return marshal(unmarshalled)
}
