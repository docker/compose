/*
   Copyright 2020 The Compose Specification Authors.

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

package template

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
)

var delimiter = "\\$"
var substitutionNamed = "[_a-z][_a-z0-9]*"

var substitutionBraced = "[_a-z][_a-z0-9]*(?::?[-?](.*}|[^}]*))?"

var patternString = fmt.Sprintf(
	"%s(?i:(?P<escaped>%s)|(?P<named>%s)|{(?P<braced>%s)}|(?P<invalid>))",
	delimiter, delimiter, substitutionNamed, substitutionBraced,
)

var defaultPattern = regexp.MustCompile(patternString)

// InvalidTemplateError is returned when a variable template is not in a valid
// format
type InvalidTemplateError struct {
	Template string
}

func (e InvalidTemplateError) Error() string {
	return fmt.Sprintf("Invalid template: %#v", e.Template)
}

// Mapping is a user-supplied function which maps from variable names to values.
// Returns the value as a string and a bool indicating whether
// the value is present, to distinguish between an empty string
// and the absence of a value.
type Mapping func(string) (string, bool)

// SubstituteFunc is a user-supplied function that apply substitution.
// Returns the value as a string, a bool indicating if the function could apply
// the substitution and an error.
type SubstituteFunc func(string, Mapping) (string, bool, error)

// SubstituteWith substitute variables in the string with their values.
// It accepts additional substitute function.
func SubstituteWith(template string, mapping Mapping, pattern *regexp.Regexp, subsFuncs ...SubstituteFunc) (string, error) {
	if len(subsFuncs) == 0 {
		subsFuncs = getDefaultSortedSubstitutionFunctions(template)
	}
	var err error
	result := pattern.ReplaceAllStringFunc(template, func(substring string) string {
		closingBraceIndex := getFirstBraceClosingIndex(substring)
		rest := ""
		if closingBraceIndex > -1 {
			rest = substring[closingBraceIndex+1:]
			substring = substring[0 : closingBraceIndex+1]
		}

		matches := pattern.FindStringSubmatch(substring)
		groups := matchGroups(matches, pattern)
		if escaped := groups["escaped"]; escaped != "" {
			return escaped
		}

		braced := false
		substitution := groups["named"]
		if substitution == "" {
			substitution = groups["braced"]
			braced = true
		}

		if substitution == "" {
			err = &InvalidTemplateError{Template: template}
			return ""
		}

		if braced {
			for _, f := range subsFuncs {
				var (
					value   string
					applied bool
				)
				value, applied, err = f(substitution, mapping)
				if err != nil {
					return ""
				}
				if !applied {
					continue
				}
				interpolatedNested, err := SubstituteWith(rest, mapping, pattern, subsFuncs...)
				if err != nil {
					return ""
				}
				return value + interpolatedNested
			}
		}

		value, ok := mapping(substitution)
		if !ok {
			logrus.Warnf("The %q variable is not set. Defaulting to a blank string.", substitution)
		}
		return value
	})

	return result, err
}

func getDefaultSortedSubstitutionFunctions(template string, fns ...SubstituteFunc) []SubstituteFunc {
	hyphenIndex := strings.IndexByte(template, '-')
	questionIndex := strings.IndexByte(template, '?')
	if hyphenIndex < 0 || hyphenIndex > questionIndex {
		return []SubstituteFunc{
			requiredNonEmpty,
			required,
			softDefault,
			hardDefault,
		}
	}
	return []SubstituteFunc{
		softDefault,
		hardDefault,
		requiredNonEmpty,
		required,
	}
}

func getFirstBraceClosingIndex(s string) int {
	openVariableBraces := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '}' {
			openVariableBraces--
			if openVariableBraces == 0 {
				return i
			}
		}
		if strings.HasPrefix(s[i:], "${") {
			openVariableBraces++
			i++
		}
	}
	return -1
}

// Substitute variables in the string with their values
func Substitute(template string, mapping Mapping) (string, error) {
	return SubstituteWith(template, mapping, defaultPattern)
}

// ExtractVariables returns a map of all the variables defined in the specified
// composefile (dict representation) and their default value if any.
func ExtractVariables(configDict map[string]interface{}, pattern *regexp.Regexp) map[string]Variable {
	if pattern == nil {
		pattern = defaultPattern
	}
	return recurseExtract(configDict, pattern)
}

func recurseExtract(value interface{}, pattern *regexp.Regexp) map[string]Variable {
	m := map[string]Variable{}

	switch value := value.(type) {
	case string:
		if values, is := extractVariable(value, pattern); is {
			for _, v := range values {
				m[v.Name] = v
			}
		}
	case map[string]interface{}:
		for _, elem := range value {
			submap := recurseExtract(elem, pattern)
			for key, value := range submap {
				m[key] = value
			}
		}

	case []interface{}:
		for _, elem := range value {
			if values, is := extractVariable(elem, pattern); is {
				for _, v := range values {
					m[v.Name] = v
				}
			}
		}
	}

	return m
}

type Variable struct {
	Name         string
	DefaultValue string
	Required     bool
}

func extractVariable(value interface{}, pattern *regexp.Regexp) ([]Variable, bool) {
	sValue, ok := value.(string)
	if !ok {
		return []Variable{}, false
	}
	matches := pattern.FindAllStringSubmatch(sValue, -1)
	if len(matches) == 0 {
		return []Variable{}, false
	}
	values := []Variable{}
	for _, match := range matches {
		groups := matchGroups(match, pattern)
		if escaped := groups["escaped"]; escaped != "" {
			continue
		}
		val := groups["named"]
		if val == "" {
			val = groups["braced"]
		}
		name := val
		var defaultValue string
		var required bool
		switch {
		case strings.Contains(val, ":?"):
			name, _ = partition(val, ":?")
			required = true
		case strings.Contains(val, "?"):
			name, _ = partition(val, "?")
			required = true
		case strings.Contains(val, ":-"):
			name, defaultValue = partition(val, ":-")
		case strings.Contains(val, "-"):
			name, defaultValue = partition(val, "-")
		}
		values = append(values, Variable{
			Name:         name,
			DefaultValue: defaultValue,
			Required:     required,
		})
	}
	return values, len(values) > 0
}

// Soft default (fall back if unset or empty)
func softDefault(substitution string, mapping Mapping) (string, bool, error) {
	sep := ":-"
	if !strings.Contains(substitution, sep) {
		return "", false, nil
	}
	name, defaultValue := partition(substitution, sep)
	defaultValue, err := Substitute(defaultValue, mapping)
	if err != nil {
		return "", false, err
	}
	value, ok := mapping(name)
	if !ok || value == "" {
		return defaultValue, true, nil
	}
	return value, true, nil
}

// Hard default (fall back if-and-only-if empty)
func hardDefault(substitution string, mapping Mapping) (string, bool, error) {
	sep := "-"
	if !strings.Contains(substitution, sep) {
		return "", false, nil
	}
	name, defaultValue := partition(substitution, sep)
	defaultValue, err := Substitute(defaultValue, mapping)
	if err != nil {
		return "", false, err
	}
	value, ok := mapping(name)
	if !ok {
		return defaultValue, true, nil
	}
	return value, true, nil
}

func requiredNonEmpty(substitution string, mapping Mapping) (string, bool, error) {
	return withRequired(substitution, mapping, ":?", func(v string) bool { return v != "" })
}

func required(substitution string, mapping Mapping) (string, bool, error) {
	return withRequired(substitution, mapping, "?", func(_ string) bool { return true })
}

func withRequired(substitution string, mapping Mapping, sep string, valid func(string) bool) (string, bool, error) {
	if !strings.Contains(substitution, sep) {
		return "", false, nil
	}
	name, errorMessage := partition(substitution, sep)
	errorMessage, err := Substitute(errorMessage, mapping)
	if err != nil {
		return "", false, err
	}
	value, ok := mapping(name)
	if !ok || !valid(value) {
		return "", true, &InvalidTemplateError{
			Template: fmt.Sprintf("required variable %s is missing a value: %s", name, errorMessage),
		}
	}
	return value, true, nil
}

func matchGroups(matches []string, pattern *regexp.Regexp) map[string]string {
	groups := make(map[string]string)
	for i, name := range pattern.SubexpNames()[1:] {
		groups[name] = matches[i+1]
	}
	return groups
}

// Split the string at the first occurrence of sep, and return the part before the separator,
// and the part after the separator.
//
// If the separator is not found, return the string itself, followed by an empty string.
func partition(s, sep string) (string, string) {
	if strings.Contains(s, sep) {
		parts := strings.SplitN(s, sep, 2)
		return parts[0], parts[1]
	}
	return s, ""
}
