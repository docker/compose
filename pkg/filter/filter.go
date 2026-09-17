/*
   Copyright 2026 Docker Compose CLI authors

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

// Package filter implements criteria=value expressions selecting a subset of
// a Compose application's services, so that the selection logic can be
// shared by every command operating on a subset of services.
package filter

import (
	"fmt"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
)

const (
	// CriteriaProfile selects services declaring the given profile in their
	// `profiles` attribute. Filtering on a profile implies activating it:
	// matching services are searched among all the services of the model,
	// whether or not their profile is otherwise active.
	CriteriaProfile = "profile"
	// CriteriaLabel selects services carrying the given label, expressed
	// either as a bare KEY (any value) or as KEY=VALUE.
	CriteriaLabel = "label"
)

// Expression is a single parsed criteria=value selection expression.
type Expression struct {
	Criteria string
	Value    string
}

// Filter is a set of selection expressions. Expressions with the same
// criteria are alternatives (OR); distinct criteria must all be satisfied
// (AND), following the `docker --filter` conventions.
type Filter []Expression

// Parse parses raw criteria=value expressions into a Filter.
func Parse(expressions []string) (Filter, error) {
	var f Filter
	for _, raw := range expressions {
		criteria, value, ok := strings.Cut(raw, "=")
		if !ok || value == "" {
			return nil, fmt.Errorf("invalid filter %q: must be a criteria=value expression", raw)
		}
		switch criteria {
		case CriteriaProfile:
			if value == "*" {
				return nil, fmt.Errorf("invalid filter %q: profiles must be selected explicitly", raw)
			}
			f = append(f, Expression{Criteria: criteria, Value: value})
		case CriteriaLabel:
			f = append(f, Expression{Criteria: criteria, Value: value})
		default:
			return nil, fmt.Errorf("invalid filter %q: unknown criteria %q (supported: %s, %s)", raw, criteria, CriteriaProfile, CriteriaLabel)
		}
	}
	return f, nil
}

// Profiles returns the profiles named by profile= expressions, so that
// callers can activate them before matching: filtering on a profile implies
// activating it.
func (f Filter) Profiles() []string {
	var profiles []string
	for _, e := range f {
		if e.Criteria == CriteriaProfile && !slices.Contains(profiles, e.Value) {
			profiles = append(profiles, e.Value)
		}
	}
	return profiles
}

// Match reports whether service satisfies every criteria of the filter, any
// expression of a criteria being sufficient for that criteria.
func (f Filter) Match(service types.ServiceConfig) bool {
	byCriteria := map[string]bool{}
	for _, e := range f {
		byCriteria[e.Criteria] = byCriteria[e.Criteria] || e.match(service)
	}
	for _, matched := range byCriteria {
		if !matched {
			return false
		}
	}
	return true
}

func (e Expression) match(service types.ServiceConfig) bool {
	switch e.Criteria {
	case CriteriaProfile:
		return slices.Contains(service.Profiles, e.Value)
	case CriteriaLabel:
		key, value, hasValue := strings.Cut(e.Value, "=")
		label, ok := service.Labels[key]
		if !ok {
			return false
		}
		return !hasValue || label == value
	default:
		return false
	}
}

// SelectNames returns the sorted names of the project's enabled services
// satisfying the filter. Callers are expected to have activated the profiles
// returned by [Filter.Profiles] beforehand, e.g. with
// [types.Project.WithProfiles].
func (f Filter) SelectNames(project *types.Project) []string {
	var names []string
	for name, service := range project.Services {
		if f.Match(service) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}
