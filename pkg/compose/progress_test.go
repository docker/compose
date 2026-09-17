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
	"context"
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

type runEventProcessor struct {
	operation string
	success   bool
}

func (r *runEventProcessor) Start(context.Context, string) {}

func (r *runEventProcessor) On(...api.Resource) {}

func (r *runEventProcessor) Done(operation string, success bool) {
	r.operation = operation
	r.success = success
}

func TestRunReportsSuccess(t *testing.T) {
	processor := &runEventProcessor{}

	err := Run(t.Context(), func(context.Context) error {
		return nil
	}, "test", processor)

	assert.NilError(t, err)
	assert.Equal(t, processor.operation, "test")
	assert.Check(t, processor.success)
}

func TestRunReportsFailure(t *testing.T) {
	expected := errors.New("test failure")
	processor := &runEventProcessor{}

	err := Run(t.Context(), func(context.Context) error {
		return expected
	}, "test", processor)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, processor.operation, "test")
	assert.Check(t, !processor.success)
}
