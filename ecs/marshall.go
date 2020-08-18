/*
   Copyright 2020 Docker, Inc.

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
)

func Marshall(template *cloudformation.Template) ([]byte, error) {
	raw, err := template.JSON()
	if err != nil {
		return nil, err
	}

	var unmarshalled interface{}
	if err := json.Unmarshal(raw, &unmarshalled); err != nil {
		return nil, fmt.Errorf("invalid JSON: %s", err)
	}

	if input, ok := unmarshalled.(map[string]interface{}); ok {
		if resources, ok := input["Resources"]; ok {
			for _, uresource := range resources.(map[string]interface{}) {
				if resource, ok := uresource.(map[string]interface{}); ok {
					if resource["Type"] == "AWS::ECS::TaskDefinition" {
						properties := resource["Properties"].(map[string]interface{})
						for _, def := range properties["ContainerDefinitions"].([]interface{}) {
							containerDefinition := def.(map[string]interface{})
							if strings.HasSuffix(containerDefinition["Name"].(string), "_InitContainer") {
								containerDefinition["Essential"] = "false"
							}
						}
					}
				}
			}
		}
	}

	raw, err = json.MarshalIndent(unmarshalled, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("invalid JSON: %s", err)
	}
	return raw, err
}
