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
	"testing"

	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// The post-rebuild prune of internal/devwatch reaches the engine's image
// pruner through the backend: the dangling images of the project are listed,
// and the ones the keep func spares are not removed.
func TestDevWatchBackend_RemoveDanglingImages(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	apiClient, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: make(client.Filters).
			Add("dangling", "true").
			Add("label", api.ProjectLabel+"=myProjectName"),
	}).Return(client.ImageListResult{
		Items: []image.Summary{{ID: "123"}, {ID: "456"}},
	}, nil).Times(1)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "123", client.ImageRemoveOptions{}).Times(1)

	err = devWatchBackend{engineBackend{tested.(*composeService)}}.RemoveDanglingImages(t.Context(), "myProjectName",
		func(img image.Summary) bool { return img.ID == "456" })
	assert.NilError(t, err)
}
