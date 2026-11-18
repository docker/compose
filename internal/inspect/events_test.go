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
	"errors"
	"io"
	"testing"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

func TestEventsTreatsEOFAsCompletion(t *testing.T) {
	ctrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(ctrl)
	streamErr := make(chan error, 1)
	streamErr <- io.EOF
	apiClient.EXPECT().Events(gomock.Any(), gomock.Any()).Return(client.EventsResult{
		Messages: make(chan events.Message),
		Err:      streamErr,
	})
	service := New(&fakeBackend{apiClient: apiClient})

	err := service.Events(t.Context(), "project", api.EventsOptions{
		Consumer: func(api.Event) error { return nil },
	})

	assert.NilError(t, err)
}

func TestEventsPropagatesStreamErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(ctrl)
	streamErr := make(chan error, 1)
	boom := errors.New("stream failed")
	streamErr <- boom
	apiClient.EXPECT().Events(gomock.Any(), gomock.Any()).Return(client.EventsResult{
		Messages: make(chan events.Message),
		Err:      streamErr,
	})
	service := New(&fakeBackend{apiClient: apiClient})

	err := service.Events(t.Context(), "project", api.EventsOptions{
		Consumer: func(api.Event) error { return nil },
	})

	assert.Equal(t, err, boom)
}
