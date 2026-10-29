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
	"errors"
	"testing"

	"gotest.tools/v3/assert"
)

func TestLogOptionsValidate(t *testing.T) {
	assert.NilError(t, LogOptions{}.Validate())
	assert.NilError(t, LogOptions{Services: []string{"web", "db"}}.Validate())
	assert.NilError(t, LogOptions{Services: []string{"web"}, Index: 2}.Validate())

	for _, services := range [][]string{nil, {"web", "db"}} {
		err := LogOptions{Services: services, Index: 1}.Validate()
		assert.Assert(t, errors.Is(err, ErrIncompatibleOptions), "services %v: got %v", services, err)
		var conflict *IncompatibleOptionsError
		assert.Assert(t, errors.As(err, &conflict))
		assert.DeepEqual(t, conflict.Options, []string{"Index", "Services"})
	}
}

func TestEventsOptionsValidate(t *testing.T) {
	assert.NilError(t, EventsOptions{Consumer: func(Event) error { return nil }}.Validate())

	err := EventsOptions{}.Validate()
	assert.Assert(t, errors.Is(err, ErrInvalidOption), "got %v", err)
	assert.Error(t, err, "invalid option (Consumer): a consumer of the events is required")
}

func TestPublishOptionsValidate(t *testing.T) {
	for _, version := range []OCIVersion{"", OCIVersion1_0, OCIVersion1_1} {
		assert.NilError(t, PublishOptions{OCIVersion: version}.Validate(), "version %q", version)
	}

	err := PublishOptions{OCIVersion: "1.2"}.Validate()
	assert.Assert(t, errors.Is(err, ErrInvalidOption), "got %v", err)
	assert.Error(t, err, `invalid option (OCIVersion): "1.2" is not one of 1.0, 1.1`)
}

func TestDownOptionsValidate(t *testing.T) {
	for _, mode := range []ImagePruneMode{ImagePruneNone, ImagePruneLocal, ImagePruneAll} {
		assert.NilError(t, DownOptions{Images: mode}.Validate(), "mode %q", mode)
	}

	err := DownOptions{Images: "bogus"}.Validate()
	assert.Assert(t, errors.Is(err, ErrInvalidOption), "got %v", err)
	assert.Error(t, err, `invalid option (Images): "bogus" is not one of local, all`)
}
