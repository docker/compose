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
)

// Validate checks the options of Logs. A replica is selected inside one
// service, so Index requires exactly one service in Services: with none there
// is nothing to select the replica of, and with several the replica would
// silently be looked up in the first one only. It returns nil, or an error
// matching ErrIncompatibleOptions, and Logs calls it before doing anything.
func (o LogOptions) Validate() error {
	if o.Index > 0 && len(o.Services) != 1 {
		return incompatible(fmt.Sprintf("Index selects a replica of one service, but %d services are selected", len(o.Services)),
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
		return &InvalidOptionError{Option: "Images", Value: string(o.Images), Allowed: []string{string(ImagePruneLocal), string(ImagePruneAll)}}
	}
	return nil
}
