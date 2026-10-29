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

package containerops

import (
	"context"
	"testing"

	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/inspect"
	"github.com/docker/compose/v5/pkg/api"
)

// fakeBackend serves canned containers and records how it was asked for them.
type fakeBackend struct {
	listed []container.Summary

	oneOff   inspect.OneOff
	all      bool
	services []string
	index    int
	asked    string
}

func (f *fakeBackend) APIClient() client.APIClient { return nil }
func (f *fakeBackend) MaxConcurrency() int         { return 0 }
func (f *fakeBackend) Events() api.EventProcessor  { return nil }
func (f *fakeBackend) DryRun() bool                { return false }
func (f *fakeBackend) CLI() command.Cli            { return nil }

func (f *fakeBackend) Containers(_ context.Context, _ string, oneOff inspect.OneOff, all bool, services ...string) ([]container.Summary, error) {
	f.asked = "Containers"
	f.oneOff, f.all, f.services = oneOff, all, services
	return f.listed, nil
}

func (f *fakeBackend) SpecifiedContainer(_ context.Context, _ string, oneOff inspect.OneOff, all bool, serviceName string, index int) (container.Summary, error) {
	f.asked = "SpecifiedContainer"
	f.oneOff, f.all, f.services, f.index = oneOff, all, []string{serviceName}, index
	return f.listed[0], nil
}

func webContainer(id string) container.Summary {
	return container.Summary{
		ID:     id,
		Names:  []string{"/" + id},
		Labels: map[string]string{api.ServiceLabel: "web"},
	}
}

func TestCopyTargetsTheReplicaAskedFor(t *testing.T) {
	backend := &fakeBackend{listed: []container.Summary{webContainer("web2")}}

	got, err := New(backend).listContainersTargetedForCopy(t.Context(), "proj", api.CopyOptions{Index: 2}, toService, "web")

	assert.NilError(t, err)
	assert.Equal(t, len(got), 1)
	assert.Equal(t, got[0].ID, "web2")
	assert.Equal(t, backend.asked, "SpecifiedContainer")
	assert.Equal(t, backend.index, 2)
	assert.Equal(t, backend.oneOff, inspect.OneOffExclude)
	assert.Assert(t, backend.all)
}

func TestCopyOneOffContainersOnlyWithAll(t *testing.T) {
	for _, all := range []bool{false, true} {
		backend := &fakeBackend{listed: []container.Summary{webContainer("web1")}}

		options := api.CopyOptions{All: all} //nolint:staticcheck // the deprecated All still includes the one-off containers
		_, err := New(backend).listContainersTargetedForCopy(t.Context(), "proj", options, toService, "web")

		assert.NilError(t, err)
		assert.DeepEqual(t, backend.services, []string{"web"})
		if all {
			assert.Equal(t, backend.oneOff, inspect.OneOffInclude)
		} else {
			assert.Equal(t, backend.oneOff, inspect.OneOffExclude)
		}
	}
}

func TestCopyFromAServiceReadsFromItsFirstContainerOnly(t *testing.T) {
	backend := &fakeBackend{listed: []container.Summary{webContainer("web1"), webContainer("web2")}}

	from, err := New(backend).listContainersTargetedForCopy(t.Context(), "proj", api.CopyOptions{}, fromService, "web")
	assert.NilError(t, err)
	assert.Equal(t, len(from), 1)
	assert.Equal(t, from[0].ID, "web1")

	to, err := New(backend).listContainersTargetedForCopy(t.Context(), "proj", api.CopyOptions{}, toService, "web")
	assert.NilError(t, err)
	assert.Equal(t, len(to), 2)
}

func TestCopyNeedsAContainer(t *testing.T) {
	_, err := New(&fakeBackend{}).listContainersTargetedForCopy(t.Context(), "proj", api.CopyOptions{}, toService, "web")

	assert.Error(t, err, `no container found for service "web"`)
}

func TestCopyRefusesWhatItCannotDo(t *testing.T) {
	service := New(&fakeBackend{})

	err := service.Copy(t.Context(), "proj", api.CopyOptions{Source: "web:/a", Destination: "db:/b"})
	assert.Error(t, err, "copying between services is not supported")

	err = service.Copy(t.Context(), "proj", api.CopyOptions{Source: "/a", Destination: "/b"})
	assert.Error(t, err, "unknown copy direction")
}

func TestCopyIncludeOneOff(t *testing.T) {
	for _, include := range []bool{false, true} {
		backend := &fakeBackend{listed: []container.Summary{webContainer("web1")}}

		_, err := New(backend).listContainersTargetedForCopy(t.Context(), "proj", api.CopyOptions{IncludeOneOff: include}, toService, "web")

		assert.NilError(t, err)
		if include {
			assert.Equal(t, backend.oneOff, inspect.OneOffInclude)
		} else {
			assert.Equal(t, backend.oneOff, inspect.OneOffExclude)
		}
	}
}
