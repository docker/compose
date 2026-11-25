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

package publish

import (
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v4"
)

// unresolvedWorkload is what the publish pre-checks need to know about one
// service or job, with every value exactly as written in the compose file:
// no interpolation, no type conversion.
type unresolvedWorkload struct {
	// envValues maps environment variable names to their value as written.
	// Variables declared without a value (a bare `KEY` or `KEY:`) are absent:
	// they pass through from the process environment at runtime, so nothing
	// is published for them.
	envValues map[string]string
	// hasEnvFile reports an env_file declaration with at least one entry.
	hasEnvFile bool
	// extendsFile is the extends.file path as written, or "".
	extendsFile string
}

// unresolvedComposeFile is the part of a compose file the publish pre-checks
// inspect, read straight from the YAML document.
type unresolvedComposeFile struct {
	// services and jobs are keyed by name. They are kept apart because the
	// extends walk mirrors processFile, which covers services only.
	services map[string]unresolvedWorkload
	jobs     map[string]unresolvedWorkload
	// configs maps config names to their inline content, when declared.
	configs map[string]string
}

// parseUnresolvedComposeFile reads a compose file as a plain YAML document
// rather than through compose-go's typed decoding. The typed decoding of an
// uninterpolated file fails on any non-string field holding an interpolation
// placeholder — short-syntax ports like `${PORT:-3000}:3000` don't fit
// ServicePortConfig, `${REPLICAS:-2}` is not an int, `${INTERVAL:-5s}` is not
// a duration — while every one of those is valid per the Compose
// specification, since interpolation resolves them before use. The publish
// pre-checks only inspect environment declarations, env_file, extends and
// inline config contents, and all of those are plain strings or lists the
// YAML document carries verbatim.
func parseUnresolvedComposeFile(filePath string) (*unresolvedComposeFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filePath, err)
	}

	parsed := &unresolvedComposeFile{
		services: map[string]unresolvedWorkload{},
		jobs:     map[string]unresolvedWorkload{},
		configs:  map[string]string{},
	}
	// jobs declare environment/env_file like services do: same leak surface,
	// same extraction
	for section, into := range map[string]map[string]unresolvedWorkload{
		"services": parsed.services,
		"jobs":     parsed.jobs,
	} {
		entries, ok := doc[section].(map[string]any)
		if !ok {
			continue
		}
		for name, raw := range entries {
			body, ok := raw.(map[string]any)
			if !ok {
				// an empty or otherwise non-mapping definition carries no
				// declaration the pre-checks inspect
				continue
			}
			into[name] = unresolvedWorkload{
				envValues:   envValuesAsWritten(body["environment"]),
				hasEnvFile:  declaresEnvFile(body["env_file"]),
				extendsFile: extendsFileAsWritten(body["extends"]),
			}
		}
	}
	if entries, ok := doc["configs"].(map[string]any); ok {
		for name, raw := range entries {
			body, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if content, ok := body["content"].(string); ok && content != "" {
				parsed.configs[name] = content
			}
		}
	}
	return parsed, nil
}

// envValuesAsWritten extracts environment entries in either the mapping or
// the list form, keeping values verbatim. Entries without a value are
// skipped: only the variable name is published for those, never a value.
func envValuesAsWritten(raw any) map[string]string {
	values := map[string]string{}
	switch env := raw.(type) {
	case map[string]any:
		for key, value := range env {
			if value == nil {
				continue
			}
			values[key] = scalarAsWritten(value)
		}
	case []any:
		for _, item := range env {
			entry, ok := item.(string)
			if !ok {
				continue
			}
			key, value, hasValue := strings.Cut(entry, "=")
			if !hasValue {
				continue
			}
			values[key] = value
		}
	}
	return values
}

// scalarAsWritten renders a scalar value as the string compose-go would carry
// for it once decoded.
func scalarAsWritten(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}

// declaresEnvFile reports an env_file declaration with at least one entry, in
// either the single-file or the list form.
func declaresEnvFile(raw any) bool {
	switch v := raw.(type) {
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	}
	return false
}

// extendsFileAsWritten extracts the extends.file path, when extends declares
// one. The string form of extends only names a service of the same file, so
// it yields no path.
func extendsFileAsWritten(raw any) string {
	extends, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	file, _ := extends["file"].(string)
	return file
}
