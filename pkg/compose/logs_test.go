/*
   Copyright 2022 Docker Compose CLI authors

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
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/pkg/stdcopy"
	containerType "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"

	compose "github.com/docker/compose/v5/pkg/api"
)

// newStdWriter is copied from github.com/moby/moby/daemon/internal/stdcopymux
// because NewStdWriter was moved to a daemon-internal package in moby v2 and
// is no longer publicly importable. We need it in tests to produce multiplexed
// streams that stdcopy.StdCopy can demultiplex.

const (
	stdWriterPrefixLen = 8
	stdWriterFdIndex   = 0
	stdWriterSizeIndex = 4
)

var bufPool = &sync.Pool{New: func() any { return bytes.NewBuffer(nil) }}

type stdWriter struct {
	io.Writer
	prefix byte
}

func (w *stdWriter) Write(p []byte) (int, error) {
	if w == nil || w.Writer == nil {
		return 0, errors.New("writer not instantiated")
	}
	if p == nil {
		return 0, nil
	}

	header := [stdWriterPrefixLen]byte{stdWriterFdIndex: w.prefix}
	binary.BigEndian.PutUint32(header[stdWriterSizeIndex:], uint32(len(p)))
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Write(header[:])
	buf.Write(p)

	n, err := w.Writer.Write(buf.Bytes())
	n -= stdWriterPrefixLen
	if n < 0 {
		n = 0
	}

	buf.Reset()
	bufPool.Put(buf)
	return n, err
}

func newStdWriter(w io.Writer, streamType stdcopy.StdType) io.Writer {
	return &stdWriter{
		Writer: w,
		prefix: byte(streamType),
	}
}

func TestComposeService_Logs_Demux(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	name := strings.ToLower(testProject)

	api.EXPECT().ContainerList(t.Context(), client.ContainerListOptions{
		All:     true,
		Filters: projectFilter(name).Add("label", oneOffFilter(false), hasConfigHashLabel()),
	}).Return(
		client.ContainerListResult{
			Items: []containerType.Summary{
				testContainer("service", "c", false),
			},
		},
		nil,
	)

	api.EXPECT().
		ContainerInspect(anyCancellableContext(), "c", gomock.Any()).
		Return(client.ContainerInspectResult{
			Container: containerType.InspectResponse{
				ID:     "c",
				Config: &containerType.Config{Tty: false},
			},
		}, nil)
	c1Reader, c1Writer := io.Pipe()
	t.Cleanup(func() {
		_ = c1Reader.Close()
		_ = c1Writer.Close()
	})
	c1Stdout := newStdWriter(c1Writer, stdcopy.Stdout)
	c1Stderr := newStdWriter(c1Writer, stdcopy.Stderr)
	go func() {
		_, err := c1Stdout.Write([]byte("hello stdout\n"))
		assert.NilError(t, err, "Writing to fake stdout")
		_, err = c1Stderr.Write([]byte("hello stderr\n"))
		assert.NilError(t, err, "Writing to fake stderr")
		_ = c1Writer.Close()
	}()
	api.EXPECT().ContainerLogs(anyCancellableContext(), "c", gomock.Any()).
		Return(c1Reader, nil)

	opts := compose.LogOptions{
		Project: &types.Project{
			Services: types.Services{
				"service": {Name: "service"},
			},
		},
	}

	consumer := &testLogConsumer{}
	err = tested.Logs(t.Context(), name, consumer, opts)
	assert.NilError(t, err)
	assert.DeepEqual(t, []string{"hello stdout", "hello stderr"}, consumer.LogsForContainer("c"))
}

// TestComposeService_Logs_ServiceFiltering ensures that we do not include
// logs from out-of-scope services based on the Compose file vs actual state.
//
// NOTE(milas): This test exists because each method is currently duplicating
// a lot of the project/service filtering logic. We should consider moving it
// to an earlier point in the loading process, at which point this test could
// safely be removed.
func TestComposeService_Logs_ServiceFiltering(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	name := strings.ToLower(testProject)

	api.EXPECT().ContainerList(t.Context(), client.ContainerListOptions{
		All:     true,
		Filters: projectFilter(name).Add("label", oneOffFilter(false), hasConfigHashLabel()),
	}).Return(
		client.ContainerListResult{
			Items: []containerType.Summary{
				testContainer("serviceA", "c1", false),
				testContainer("serviceA", "c2", false),
				// serviceB will be filtered out by the project definition to
				// ensure we ignore "orphan" containers
				testContainer("serviceB", "c3", false),
				testContainer("serviceC", "c4", false),
			},
		},
		nil,
	)

	for _, id := range []string{"c1", "c2", "c4"} {
		api.EXPECT().
			ContainerInspect(anyCancellableContext(), id, gomock.Any()).
			Return(
				client.ContainerInspectResult{
					Container: containerType.InspectResponse{
						ID:     id,
						Config: &containerType.Config{Tty: true},
					},
				},
				nil,
			)
		api.EXPECT().ContainerLogs(anyCancellableContext(), id, gomock.Any()).
			Return(io.NopCloser(strings.NewReader("hello "+id+"\n")), nil).
			Times(1)
	}

	// this simulates passing `--filename` with a Compose file that does NOT
	// reference `serviceB` even though it has running services for this proj
	proj := &types.Project{
		Services: types.Services{
			"serviceA": {Name: "serviceA"},
			"serviceC": {Name: "serviceC"},
		},
	}
	consumer := &testLogConsumer{}
	opts := compose.LogOptions{
		Project: proj,
	}
	err = tested.Logs(t.Context(), name, consumer, opts)
	assert.NilError(t, err)

	assert.Assert(t, is.DeepEqual([]string{"hello c1"}, consumer.LogsForContainer("c1")))
	assert.Assert(t, is.DeepEqual([]string{"hello c2"}, consumer.LogsForContainer("c2")))
	assert.Assert(t, is.Len(consumer.LogsForContainer("c3"), 0))
	assert.Assert(t, is.DeepEqual([]string{"hello c4"}, consumer.LogsForContainer("c4")))
}

type testLogConsumer struct {
	mu sync.Mutex
	// logs is keyed by container ID; values are log lines
	logs map[string][]string
}

func (l *testLogConsumer) Log(containerName, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.logs == nil {
		l.logs = make(map[string][]string)
	}
	l.logs[containerName] = append(l.logs[containerName], message)
}

func (l *testLogConsumer) Err(containerName, message string) {
	l.Log(containerName, message)
}

func (l *testLogConsumer) Status(containerName, msg string) {}

func (l *testLogConsumer) LogsForContainer(containerName string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.logs[containerName]
}
