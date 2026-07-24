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
	"sort"

	"github.com/compose-spec/compose-go/v2/template"
)

const variableExtractionValueKey = "value"

func ExtractVariables(model map[string]any) map[string]template.Variable {
	variables := map[string]template.Variable{}
	extractVariables(model, variables)
	return variables
}

func extractVariables(value any, variables map[string]template.Variable) {
	switch value := value.(type) {
	case string:
		mergeVariables(variables, template.ExtractVariables(map[string]any{variableExtractionValueKey: value}, template.DefaultPattern))
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			extractVariables(value[key], variables)
		}
	case []any:
		for _, elem := range value {
			extractVariables(elem, variables)
		}
	}
}

func mergeVariables(dst map[string]template.Variable, src map[string]template.Variable) {
	for name, variable := range src {
		current, ok := dst[name]
		if !ok {
			dst[name] = variable
			continue
		}
		current.Required = current.Required || variable.Required
		if current.DefaultValue == "" {
			current.DefaultValue = variable.DefaultValue
		}
		if current.PresenceValue == "" {
			current.PresenceValue = variable.PresenceValue
		}
		dst[name] = current
	}
}
