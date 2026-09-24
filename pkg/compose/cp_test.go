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
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v5/pkg/api"
)

// TestCopyToService_ConcurrencyIsBoundedAcrossContainers guards against the
// same unbounded-fan-out bug fixed everywhere else in this package: `compose
// cp` copying to a service fans out one goroutine per matched container via
// a bare errgroup.Group, ignoring --parallel entirely.
func TestCopyToService_ConcurrencyIsBoundedAcrossContainers(t *testing.T) {
	svc, apiClient := newTestService(t, WithMaxConcurrency(1))

	srcFile := filepath.Join(t.TempDir(), "src.txt")
	assert.NilError(t, os.WriteFile(srcFile, []byte("hello"), 0o644))

	const numContainers = 4
	var containers []container.Summary
	for i := range numContainers {
		containers = append(containers, testContainer("myservice", string(rune('a'+i)), false))
	}

	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: containers}, nil)

	apiClient.EXPECT().ContainerStatPath(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(client.ContainerStatPathResult{Stat: container.PathStat{Mode: os.ModeDir}}, nil).
		Times(numContainers)

	tracker := &peakConcurrencyTracker{}
	apiClient.EXPECT().CopyToContainer(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
			tracker.enter()
			time.Sleep(20 * time.Millisecond) // widen the window for a concurrency violation to show up
			tracker.leave()
			_, err := io.Copy(io.Discard, opts.Content)
			return client.CopyToContainerResult{}, err
		}).
		Times(numContainers)

	err := svc.copy(t.Context(), "prj", compose.CopyOptions{
		Source:      srcFile,
		Destination: "myservice:/dest",
	})
	assert.NilError(t, err)
	assert.Equal(t, tracker.Peak(), 1, "cp must never run more than maxConcurrency CopyToContainer calls at once")
}
