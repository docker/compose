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
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

func statsBody(t *testing.T, v container.StatsResponse) client.ContainerStatsResult {
	t.Helper()
	b, err := json.Marshal(v)
	assert.NilError(t, err)
	return client.ContainerStatsResult{Body: io.NopCloser(bytes.NewReader(b))}
}

// sampleResponse builds a stats sample using 50% of one CPU (out of 2) over the last interval.
func sampleResponse(memUsage, inactiveFile uint64) container.StatsResponse {
	var v container.StatsResponse
	v.PreCPUStats.CPUUsage.TotalUsage = 1_000
	v.PreCPUStats.SystemUsage = 10_000
	v.CPUStats.CPUUsage.TotalUsage = 1_500
	v.CPUStats.SystemUsage = 12_000
	v.CPUStats.OnlineCPUs = 2
	v.MemoryStats.Usage = memUsage
	v.MemoryStats.Limit = 10_000
	v.MemoryStats.Stats = map[string]uint64{"inactive_file": inactiveFile}
	v.Networks = map[string]container.NetworkStats{
		"eth0": {RxBytes: 10, TxBytes: 20},
		"eth1": {RxBytes: 1, TxBytes: 2},
	}
	v.BlkioStats.IoServiceBytesRecursive = []container.BlkioStatEntry{
		{Op: "Read", Value: 100},
		{Op: "Write", Value: 200},
		{Op: "Total", Value: 300},
	}
	v.PidsStats.Current = 3
	return v
}

func TestContainerUsage(t *testing.T) {
	u := containerUsage(sampleResponse(1000, 400))

	// (cpuDelta=500 / systemDelta=2000) * 2 cpus * 100
	assert.Equal(t, u.cpuPercent, 50.0)
	// page cache (inactive_file) is not counted as usage
	assert.Equal(t, u.memory, 600.0)
	assert.Equal(t, u.netRx, 11.0)
	assert.Equal(t, u.netTx, 22.0)
	assert.Equal(t, u.blockRead, 100.0)
	assert.Equal(t, u.blockWrite, 200.0)
	assert.Equal(t, u.pids, uint64(3))
}

func TestContainerUsageWithoutSystemDelta(t *testing.T) {
	v := sampleResponse(1000, 0)
	v.PreCPUStats.SystemUsage = v.CPUStats.SystemUsage // no previous sample
	assert.Equal(t, containerUsage(v).cpuPercent, 0.0)
}

func projectContainer(name, project, state string) container.Summary {
	return container.Summary{
		ID: name + "-id",
		// the canonical name comes with a link alias, which must not be picked
		Names:  []string{"/linked_by/" + name, "/" + name},
		Labels: map[string]string{api.ProjectLabel: project},
		State:  container.ContainerState(state),
	}
}

func TestCollectContainerStats(t *testing.T) {
	apiClient := mocks.NewMockAPIClient(gomock.NewController(t))
	filters := client.Filters{}.Add("label", api.ProjectLabel)

	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, opts client.ContainerListOptions) (client.ContainerListResult, error) {
			assert.Assert(t, opts.All)
			assert.DeepEqual(t, opts.Filters, filters)
			return client.ContainerListResult{Items: []container.Summary{
				projectContainer("beta-1", "beta", "running"),
				projectContainer("alpha-2", "alpha", "running"),
				projectContainer("alpha-1", "alpha", "running"),
				projectContainer("alpha-3", "alpha", "exited"),
				projectContainer("gone-1", "gone", "running"),
			}}, nil
		})
	// only running containers are sampled, and the previous sample is needed for the CPU delta
	for id, mem := range map[string]uint64{"alpha-1-id": 1000, "alpha-2-id": 3000, "beta-1-id": 500} {
		apiClient.EXPECT().ContainerStats(gomock.Any(), id, client.ContainerStatsOptions{IncludePreviousSample: true}).
			Return(statsBody(t, sampleResponse(mem, 0)), nil)
	}
	// removed between the list and the stats call: not an error
	apiClient.EXPECT().ContainerStats(gomock.Any(), "gone-1-id", gomock.Any()).
		Return(client.ContainerStatsResult{}, errdefs.ErrNotFound)

	rows, err := collectContainerStats(t.Context(), apiClient, true, filters)
	assert.NilError(t, err)

	// sorted by project, then by container name
	var names []string
	for _, r := range rows {
		names = append(names, r.project+"/"+r.name)
	}
	assert.DeepEqual(t, names, []string{"alpha/alpha-1", "alpha/alpha-2", "alpha/alpha-3", "beta/beta-1", "gone/gone-1"})

	assert.Equal(t, rows[0].memory, 1000.0)
	assert.Equal(t, rows[0].memLimit, 10_000.0)
	assert.Equal(t, rows[1].memory, 3000.0)
	// stopped and removed containers are still listed, without usage
	assert.Equal(t, rows[2].memory, 0.0)
	assert.Equal(t, rows[4].memory, 0.0)
}

func TestAggregateByProject(t *testing.T) {
	rows := []containerStats{
		{project: "alpha", name: "alpha-1", usage: usage{cpuPercent: 50, memory: 1000, netRx: 11, pids: 3}},
		{project: "alpha", name: "alpha-2", usage: usage{cpuPercent: 50, memory: 3000, netRx: 11, pids: 3}},
		{project: "alpha", name: "alpha-3"},
		{project: "beta", name: "beta-1", usage: usage{memory: 500}},
	}

	stats := aggregateByProject(rows)

	assert.Equal(t, len(stats), 2)
	assert.Equal(t, stats[0].name, "alpha")
	assert.Equal(t, stats[0].containers, 3)
	assert.Equal(t, stats[0].cpuPercent, 100.0)
	assert.Equal(t, stats[0].memory, 4000.0)
	assert.Equal(t, stats[0].netRx, 22.0)
	assert.Equal(t, stats[0].pids, uint64(6))
	assert.Equal(t, stats[1].name, "beta")
	assert.Equal(t, stats[1].containers, 1)
	assert.Equal(t, stats[1].memory, 500.0)
}

// decodeJSONLines decodes the output of the json format, which is one object per line like `docker stats`.
func decodeJSONLines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var decoded []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var obj map[string]any
		assert.NilError(t, json.Unmarshal([]byte(line), &obj), line)
		decoded = append(decoded, obj)
	}
	return decoded
}

func TestPrintContainerStats(t *testing.T) {
	rows := []containerStats{{
		project:  "alpha",
		name:     "alpha-1",
		memLimit: 10_000,
		usage:    usage{cpuPercent: 50, memory: 600, netRx: 11, netTx: 22, blockRead: 100, blockWrite: 200, pids: 3},
	}}

	var table bytes.Buffer
	assert.NilError(t, printContainerStats(&table, "table", rows))
	lines := strings.Split(strings.TrimSpace(table.String()), "\n")
	assert.Equal(t, len(lines), 2)
	assert.DeepEqual(t, strings.Fields(lines[0]), []string{"PROJECT", "NAME", "CPU", "%", "MEM", "USAGE", "/", "LIMIT", "MEM", "%", "NET", "I/O", "BLOCK", "I/O", "PIDS"})
	assert.DeepEqual(t, strings.Fields(lines[1]), []string{"alpha", "alpha-1", "50.00%", "600B", "/", "9.766KiB", "6.00%", "11B", "/", "22B", "100B", "/", "200B", "3"})

	var out bytes.Buffer
	assert.NilError(t, printContainerStats(&out, "json", append(rows, containerStats{project: "beta", name: "beta-1"})))
	decoded := decodeJSONLines(t, out.String())
	assert.Equal(t, len(decoded), 2)
	assert.Equal(t, decoded[0]["Project"], "alpha")
	assert.Equal(t, decoded[0]["Name"], "alpha-1")
	assert.Equal(t, decoded[0]["MemPerc"], "6.00%")
	assert.Equal(t, decoded[1]["Name"], "beta-1")

	// nothing to report, nothing printed
	out.Reset()
	assert.NilError(t, printContainerStats(&out, "json", nil))
	assert.Equal(t, out.String(), "")
}

func TestPrintProjectStats(t *testing.T) {
	stats := []projectStats{{name: "alpha", containers: 2, usage: usage{cpuPercent: 100, memory: 4000, pids: 6}}}

	var table bytes.Buffer
	assert.NilError(t, printProjectStats(&table, "table", stats))
	lines := strings.Split(strings.TrimSpace(table.String()), "\n")
	assert.Equal(t, len(lines), 2)
	assert.DeepEqual(t, strings.Fields(lines[0])[:2], []string{"PROJECT", "CONTAINERS"})
	assert.DeepEqual(t, strings.Fields(lines[1])[:3], []string{"alpha", "2", "100.00%"})

	var out bytes.Buffer
	assert.NilError(t, printProjectStats(&out, "json", append(stats, projectStats{name: "beta", containers: 1})))
	decoded := decodeJSONLines(t, out.String())
	assert.Equal(t, len(decoded), 2)
	assert.Equal(t, decoded[1]["Project"], "beta")
	assert.Equal(t, decoded[0]["Project"], "alpha")
	assert.Equal(t, decoded[0]["Containers"], float64(2))
	_, hasName := decoded[0]["Name"]
	assert.Assert(t, !hasName)
}

func TestProjectFilters(t *testing.T) {
	assert.DeepEqual(t, projectFilters("foo", nil), client.Filters{}.Add("label", api.ProjectLabel+"=foo"))
	assert.DeepEqual(t, projectFilters("foo", []string{"web"}),
		client.Filters{}.Add("label", api.ProjectLabel+"=foo", api.ServiceLabel+"=web"))
}

func TestValidateStatsProjects(t *testing.T) {
	err := validateStatsProjects(statsOptions{ProjectOptions: &ProjectOptions{ProjectName: "foo"}, allProjects: true}, nil)
	assert.ErrorContains(t, err, "--project-name")

	// whole projects are reported, never a single service: the error names the flags that were given
	tests := []struct {
		opts    statsOptions
		message string
	}{
		{statsOptions{allProjects: true}, "--all-projects report on whole projects and cannot be used with a SERVICE"},
		{statsOptions{aggregate: true}, "--aggregate report on whole projects and cannot be used with a SERVICE"},
		{statsOptions{allProjects: true, aggregate: true}, "--all-projects and --aggregate report on whole projects and cannot be used with a SERVICE"},
	}
	for _, tt := range tests {
		tt.opts.ProjectOptions = &ProjectOptions{}
		assert.Error(t, validateStatsProjects(tt.opts, []string{"web"}), tt.message)
		assert.NilError(t, validateStatsProjects(tt.opts, nil))
	}

	// --aggregate alone reports on the current project, which -p selects
	assert.NilError(t, validateStatsProjects(statsOptions{ProjectOptions: &ProjectOptions{ProjectName: "foo"}, aggregate: true}, nil))
}

// An unsupported format must fail before anything is listed nor sampled: the mocks expect no call at all.
func TestStatsProjectsRejectsUnsupportedFormatUpFront(t *testing.T) {
	ctrl := gomock.NewController(t)
	cli := mocks.NewMockCli(ctrl)

	for _, aggregate := range []bool{false, true} {
		opts := statsOptions{ProjectOptions: &ProjectOptions{}, allProjects: true, aggregate: aggregate, format: "{{.Name}}"}
		err := runStatsProjects(t.Context(), cli, opts, nil)
		assert.ErrorIs(t, err, api.ErrParsingFailed)
	}
}
