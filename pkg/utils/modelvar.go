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

package utils

import "strings"

// GetModelVariables generates default model and endpoint variable names from a model reference.
// It converts the model reference to uppercase and replaces hyphens with underscores.
// Returns modelVariable (e.g., "AI_RUNNER_MODEL") and endpointVariable (e.g., "AI_RUNNER_URL").
func GetModelVariables(modelRef string) (modelVariable, endpointVariable string) {
	prefix := strings.ReplaceAll(strings.ToUpper(modelRef), "-", "_")
	return prefix + "_MODEL", prefix + "_URL"
}
