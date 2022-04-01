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

package schema

import (
	"fmt"
	"strings"
	"time"

	"github.com/xeipuuv/gojsonschema"

	// Enable support for embedded static resources
	_ "embed"
)

type portsFormatChecker struct{}

func (checker portsFormatChecker) IsFormat(input interface{}) bool {
	// TODO: implement this
	return true
}

type durationFormatChecker struct{}

func (checker durationFormatChecker) IsFormat(input interface{}) bool {
	value, ok := input.(string)
	if !ok {
		return false
	}
	_, err := time.ParseDuration(value)
	return err == nil
}

func init() {
	gojsonschema.FormatCheckers.Add("expose", portsFormatChecker{})
	gojsonschema.FormatCheckers.Add("ports", portsFormatChecker{})
	gojsonschema.FormatCheckers.Add("duration", durationFormatChecker{})
}

// Schema is the compose-spec JSON schema
//go:embed compose-spec.json
var Schema string

// Validate uses the jsonschema to validate the configuration
func Validate(config map[string]interface{}) error {
	schemaLoader := gojsonschema.NewStringLoader(Schema)
	dataLoader := gojsonschema.NewGoLoader(config)

	result, err := gojsonschema.Validate(schemaLoader, dataLoader)
	if err != nil {
		return err
	}

	if !result.Valid() {
		return toError(result)
	}

	return nil
}

func toError(result *gojsonschema.Result) error {
	err := getMostSpecificError(result.Errors())
	return err
}

const (
	jsonschemaOneOf = "number_one_of"
	jsonschemaAnyOf = "number_any_of"
)

func getDescription(err validationError) string {
	switch err.parent.Type() {
	case "invalid_type":
		if expectedType, ok := err.parent.Details()["expected"].(string); ok {
			return fmt.Sprintf("must be a %s", humanReadableType(expectedType))
		}
	case jsonschemaOneOf, jsonschemaAnyOf:
		if err.child == nil {
			return err.parent.Description()
		}
		return err.child.Description()
	}
	return err.parent.Description()
}

func humanReadableType(definition string) string {
	if definition[0:1] == "[" {
		allTypes := strings.Split(definition[1:len(definition)-1], ",")
		for i, t := range allTypes {
			allTypes[i] = humanReadableType(t)
		}
		return fmt.Sprintf(
			"%s or %s",
			strings.Join(allTypes[0:len(allTypes)-1], ", "),
			allTypes[len(allTypes)-1],
		)
	}
	if definition == "object" {
		return "mapping"
	}
	if definition == "array" {
		return "list"
	}
	return definition
}

type validationError struct {
	parent gojsonschema.ResultError
	child  gojsonschema.ResultError
}

func (err validationError) Error() string {
	description := getDescription(err)
	return fmt.Sprintf("%s %s", err.parent.Field(), description)
}

func getMostSpecificError(errors []gojsonschema.ResultError) validationError {
	mostSpecificError := 0
	for i, err := range errors {
		if specificity(err) > specificity(errors[mostSpecificError]) {
			mostSpecificError = i
			continue
		}

		if specificity(err) == specificity(errors[mostSpecificError]) {
			// Invalid type errors win in a tie-breaker for most specific field name
			if err.Type() == "invalid_type" && errors[mostSpecificError].Type() != "invalid_type" {
				mostSpecificError = i
			}
		}
	}

	if mostSpecificError+1 == len(errors) {
		return validationError{parent: errors[mostSpecificError]}
	}

	switch errors[mostSpecificError].Type() {
	case "number_one_of", "number_any_of":
		return validationError{
			parent: errors[mostSpecificError],
			child:  errors[mostSpecificError+1],
		}
	default:
		return validationError{parent: errors[mostSpecificError]}
	}
}

func specificity(err gojsonschema.ResultError) int {
	return len(strings.Split(err.Field(), "."))
}
