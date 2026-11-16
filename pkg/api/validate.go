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

package api

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// InvalidOptionError reports an option set to a value it does not accept, or a
// required option left unset. It is returned, before any side effect, by the
// Validate method of the options of an operation, and therefore by the
// operation itself.
//
// It matches ErrInvalidOption with errors.Is, and callers needing the details
// can retrieve it with errors.As.
type InvalidOptionError struct {
	// Option names the option field, as a Go selector relative to the options
	// type that was validated (e.g. "Images").
	Option string
	// Value is the value that was set.
	Value string
	// Allowed lists the values the option accepts, when it is a closed set.
	Allowed []string
	// Reason explains what is wrong when there is no value to blame, for
	// instance a required option left unset. It replaces Value and Allowed in
	// the message.
	Reason string
}

func (e *InvalidOptionError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("%s (%s): %s", ErrInvalidOption, e.Option, e.Reason)
	}
	if len(e.Allowed) == 0 {
		return fmt.Sprintf("%s (%s): %q is not a valid value", ErrInvalidOption, e.Option, e.Value)
	}
	allowed := make([]string, len(e.Allowed))
	for i, a := range e.Allowed {
		allowed[i] = strconv.Quote(a)
	}
	return fmt.Sprintf("%s (%s): %q is not one of %s", ErrInvalidOption, e.Option, e.Value, strings.Join(allowed, ", "))
}

// Unwrap makes errors.Is(err, ErrInvalidOption) hold.
func (e *InvalidOptionError) Unwrap() error {
	return ErrInvalidOption
}

// Validate checks the options of Logs. A replica is selected inside one
// service, so Index requires exactly one service in Services: with none there
// is nothing to select the replica of, and with several the replica would
// silently be looked up in the first one only. It returns nil, or an error
// matching ErrIncompatibleOptions, and Logs calls it before doing anything.
func (o LogOptions) Validate() error {
	if o.Index > 0 && len(o.Services) != 1 {
		return incompatible("Index selects a replica of one service, but "+servicesSelected(len(o.Services)),
			"Index", "Services")
	}
	return nil
}

// Validate checks the options of Events: the Consumer receives every event and
// must be set. It returns nil, or an error matching ErrInvalidOption, and
// Events calls it before doing anything.
func (o EventsOptions) Validate() error {
	if o.Consumer == nil {
		return &InvalidOptionError{Option: "Consumer", Reason: "a consumer of the events is required"}
	}
	return nil
}

// Validate checks the options of Publish: OCIVersion must be empty (the
// version is then determined from the registry) or one of the OCIVersion*
// constants. It returns nil, or an error matching ErrInvalidOption, and
// Publish calls it before pushing anything.
func (o PublishOptions) Validate() error {
	allowed := []string{string(OCIVersion1_0), string(OCIVersion1_1)}
	if o.OCIVersion != "" && !slices.Contains(allowed, string(o.OCIVersion)) {
		return &InvalidOptionError{Option: "OCIVersion", Value: string(o.OCIVersion), Allowed: allowed}
	}
	return nil
}

// Validate checks the options of Down: Images must be one of the ImagePrune*
// values. It returns nil, or an error matching ErrInvalidOption, and Down
// calls it before touching a single resource.
func (o DownOptions) Validate() error {
	if !o.Images.Valid() {
		return &InvalidOptionError{Option: "Images", Value: string(o.Images), Allowed: []string{string(ImagePruneNone), string(ImagePruneLocal), string(ImagePruneAll)}}
	}
	return nil
}

func servicesSelected(n int) string {
	if n == 0 {
		return "no service is selected"
	}
	return fmt.Sprintf("%d services are selected", n)
}
