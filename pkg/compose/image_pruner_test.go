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

package compose

import (
	"testing"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
)

// TestRemoveDanglingImages_FiltersKeepsAndToleratesFailures guards the
// shared helper used by both `down --rmi` and `watch --prune`: images the
// keep predicate spares must never be removed; an image already gone (a
// benign race with something else removing it concurrently) is tolerated
// and not reported as a failure; a genuine removal error doesn't stop the
// others, but is joined into the returned error (preserving the actual
// daemon error, not just the image ID) for callers that need to surface it.
func TestRemoveDanglingImages_FiltersKeepsAndToleratesFailures(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, cli := prepareMocks(mockCtrl)
	svcIface, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := svcIface.(*composeService)

	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:keep"},
		{ID: "sha256:removed"},
		{ID: "sha256:already-gone"},
		{ID: "sha256:fails"},
	}}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:removed", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:already-gone", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrNotFound.WithMessage("already removed"))
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:fails", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrConflict.WithMessage("image is being used"))
	// no expectation for "sha256:keep" — a call to ImageRemove for it fails the test

	keep := func(img image.Summary) bool { return img.ID == "sha256:keep" }
	removed, err := svc.removeDanglingImages(t.Context(), "prj", keep)
	assert.DeepEqual(t, removed, []string{"sha256:removed"})
	assert.ErrorContains(t, err, "sha256:fails")
	assert.ErrorContains(t, err, "image is being used")
}

// TestRemoveDanglingImages_NoneFound guards that an empty dangling-image
// list is a no-op: no removal calls, no error.
func TestRemoveDanglingImages_NoneFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, cli := prepareMocks(mockCtrl)
	svcIface, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := svcIface.(*composeService)

	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{}, nil)

	removed, err := svc.removeDanglingImages(t.Context(), "prj", func(image.Summary) bool { return false })
	assert.NilError(t, err)
	assert.Equal(t, len(removed), 0)
}
