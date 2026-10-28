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

package inspect

import (
	"context"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// fakeBackend serves canned containers and records how it was asked for them.
type fakeBackend struct {
	apiClient client.APIClient
	listed    []container.Summary

	oneOff   OneOff
	all      bool
	services []string
	index    int
}

func (f *fakeBackend) APIClient() client.APIClient { return f.apiClient }
func (f *fakeBackend) MaxConcurrency() int         { return 2 }

func (f *fakeBackend) Containers(_ context.Context, _ string, oneOff OneOff, all bool, services ...string) ([]container.Summary, error) {
	f.oneOff, f.all, f.services = oneOff, all, services
	return f.listed, nil
}

func (f *fakeBackend) SpecifiedContainer(_ context.Context, _ string, oneOff OneOff, all bool, serviceName string, index int) (container.Summary, error) {
	f.oneOff, f.all, f.services, f.index = oneOff, all, []string{serviceName}, index
	return f.listed[0], nil
}

func summary(service, id string) container.Summary {
	return container.Summary{
		ID:     id,
		Names:  []string{"/" + id},
		Labels: map[string]string{api.ServiceLabel: service},
	}
}

func TestPsOneOffContainersOnlyWithAll(t *testing.T) {
	for _, all := range []bool{false, true} {
		ctrl := gomock.NewController(t)
		apiClient := mocks.NewMockAPIClient(ctrl)
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "web1", gomock.Any()).Return(client.ContainerInspectResult{}, nil)
		backend := &fakeBackend{apiClient: apiClient, listed: []container.Summary{summary("web", "web1")}}

		got, err := New(backend).Ps(t.Context(), "proj", api.PsOptions{All: all}) //nolint:staticcheck // the deprecated All still sets both switches

		assert.NilError(t, err)
		assert.Equal(t, len(got), 1)
		assert.Equal(t, got[0].Service, "web")
		assert.Equal(t, backend.all, all)
		if all {
			assert.Equal(t, backend.oneOff, OneOffInclude)
		} else {
			assert.Equal(t, backend.oneOff, OneOffExclude)
		}
	}
}

func TestPsKeepsTheRequestedServices(t *testing.T) {
	ctrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(ctrl)
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db1", gomock.Any()).Return(client.ContainerInspectResult{}, nil)
	backend := &fakeBackend{apiClient: apiClient, listed: []container.Summary{summary("web", "web1"), summary("db", "db1")}}

	got, err := New(backend).Ps(t.Context(), "proj", api.PsOptions{Services: []string{"db"}})

	assert.NilError(t, err)
	assert.Equal(t, len(got), 1)
	assert.Equal(t, got[0].ID, "db1")
}

func TestPortsAsksForTheReplica(t *testing.T) {
	backend := &fakeBackend{listed: []container.Summary{summary("web", "web2")}}

	_, err := New(backend).Ports(t.Context(), "proj", "web", 0, api.PortOptions{Index: 2})

	assert.NilError(t, err)
	assert.DeepEqual(t, backend.services, []string{"web"})
	assert.Equal(t, backend.index, 2)
	assert.Equal(t, backend.oneOff, OneOffInclude)
	assert.Equal(t, backend.all, false)
}

// IncludeStopped and IncludeOneOff are independent: the stopped containers can
// be listed without the one-off ones, and the other way round.
func TestPsIncludeStoppedAndIncludeOneOff(t *testing.T) {
	tests := []struct {
		name           string
		options        api.PsOptions
		expectedAll    bool
		expectedOneOff OneOff
	}{
		{name: "defaults", options: api.PsOptions{}, expectedAll: false, expectedOneOff: OneOffExclude},
		{name: "stopped only", options: api.PsOptions{IncludeStopped: true}, expectedAll: true, expectedOneOff: OneOffExclude},
		{name: "one-off only", options: api.PsOptions{IncludeOneOff: true}, expectedAll: false, expectedOneOff: OneOffInclude},
		{name: "both", options: api.PsOptions{IncludeStopped: true, IncludeOneOff: true}, expectedAll: true, expectedOneOff: OneOffInclude},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			apiClient := mocks.NewMockAPIClient(ctrl)
			apiClient.EXPECT().ContainerInspect(gomock.Any(), "web1", gomock.Any()).Return(client.ContainerInspectResult{}, nil)
			backend := &fakeBackend{apiClient: apiClient, listed: []container.Summary{summary("web", "web1")}}

			_, err := New(backend).Ps(t.Context(), "proj", tt.options)

			assert.NilError(t, err)
			assert.Equal(t, backend.all, tt.expectedAll)
			assert.Equal(t, backend.oneOff, tt.expectedOneOff)
		})
	}
}
