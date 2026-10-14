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
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/docker/cli/cli/command"
	"github.com/docker/go-units"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/cmd/formatter"
	"github.com/docker/compose/v5/internal/naming"
	"github.com/docker/compose/v5/pkg/api"
)

const statsProjectsRefreshInterval = 2 * time.Second

// statsConcurrency bounds the samples collected at the same time. The daemon needs about a second to sample a
// container, so a refresh of n containers lasts at least n/statsConcurrency seconds.
const statsConcurrency = 32

// usage is the resource consumption of one or several containers.
type usage struct {
	cpuPercent float64
	memory     float64
	netRx      float64
	netTx      float64
	blockRead  float64
	blockWrite float64
	pids       uint64
}

func (u *usage) add(o usage) {
	u.cpuPercent += o.cpuPercent
	u.memory += o.memory
	u.netRx += o.netRx
	u.netTx += o.netTx
	u.blockRead += o.blockRead
	u.blockWrite += o.blockWrite
	u.pids += o.pids
}

// containerStats is the resource consumption of a container.
type containerStats struct {
	project  string
	name     string
	memLimit float64
	usage
}

// projectStats is the resource consumption of all the containers of a project.
type projectStats struct {
	name       string
	containers int
	usage
}

type containerStatsView struct {
	Project  string
	Name     string
	CPUPerc  string
	MemUsage string
	MemPerc  string
	NetIO    string
	BlockIO  string
	PIDs     uint64
}

type projectStatsView struct {
	Project    string
	Containers int
	CPUPerc    string
	MemUsage   string
	NetIO      string
	BlockIO    string
	PIDs       uint64
}

func validateStatsProjects(opts statsOptions, services []string) error {
	if len(services) > 0 {
		var flags []string
		if opts.allProjects {
			flags = append(flags, "--all-projects")
		}
		if opts.aggregate {
			flags = append(flags, "--aggregate")
		}
		return fmt.Errorf("%s report on whole projects and cannot be used with a SERVICE", strings.Join(flags, " and "))
	}
	if opts.allProjects && opts.ProjectOptions.ProjectName != "" {
		return errors.New("--all-projects reports on all projects and cannot be used with --project-name")
	}
	return nil
}

func projectFilters(projectName string, services []string) client.Filters {
	f := client.Filters{}.Add("label", fmt.Sprintf("%s=%s", api.ProjectLabel, projectName))
	if len(services) > 0 {
		f.Add("label", fmt.Sprintf("%s=%s", api.ServiceLabel, services[0]))
	}
	return f
}

// runStatsProjects serves `stats --all-projects` and `stats --aggregate`: the per container or per project
// view of either all the projects or the current one.
func runStatsProjects(ctx context.Context, dockerCli command.Cli, opts statsOptions, services []string) error {
	if err := validateStatsProjects(opts, services); err != nil {
		return err
	}
	var filters client.Filters
	if opts.allProjects {
		filters = client.Filters{}.Add("label", api.ProjectLabel)
	} else {
		name, err := opts.ProjectOptions.toProjectName(ctx, dockerCli)
		if err != nil {
			return err
		}
		filters = projectFilters(name, nil)
	}

	render := func(out io.Writer, rows []containerStats) error {
		if opts.aggregate {
			return printProjectStats(out, opts.format, aggregateByProject(rows))
		}
		return printContainerStats(out, opts.format, rows)
	}
	// reject an unsupported format before sampling anything
	if err := render(io.Discard, nil); err != nil {
		return err
	}

	out := dockerCli.Out()
	for {
		rows, err := collectContainerStats(ctx, dockerCli.Client(), opts.all, filters)
		if err != nil {
			return err
		}
		if !opts.noStream && out.IsTerminal() {
			_, _ = io.WriteString(out, "\033[H\033[J")
		}
		if err := render(out, rows); err != nil || opts.noStream {
			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(statsProjectsRefreshInterval):
		}
	}
}

// collectContainerStats samples the running containers matching filters. Containers are returned sorted by
// project, then by name; the ones not running are returned without usage.
func collectContainerStats(ctx context.Context, apiClient client.ContainerAPIClient, all bool, filters client.Filters) ([]containerStats, error) {
	list, err := apiClient.ContainerList(ctx, client.ContainerListOptions{All: all, Filters: filters})
	if err != nil {
		return nil, err
	}

	rows := make([]containerStats, len(list.Items))
	var eg errgroup.Group
	eg.SetLimit(statsConcurrency)
	for i, c := range list.Items {
		rows[i] = containerStats{project: c.Labels[api.ProjectLabel], name: naming.CanonicalContainerName(c)}
		if c.State != container.StateRunning {
			continue
		}
		eg.Go(func() error {
			// a container that cannot be sampled (removed since it was listed, daemon failure) is shown without
			// usage rather than stopping the refresh of the others
			if v, err := sampleStats(ctx, apiClient, c.ID); err == nil {
				rows[i].usage = containerUsage(v)
				rows[i].memLimit = float64(v.MemoryStats.Limit)
			}
			return nil
		})
	}
	_ = eg.Wait() // the goroutines never fail
	// an interrupted sampling is not a list of containers without usage
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	slices.SortFunc(rows, func(a, b containerStats) int {
		return cmp.Or(strings.Compare(a.project, b.project), strings.Compare(a.name, b.name))
	})
	return rows, nil
}

// aggregateByProject sums the usage of the containers of each project. rows must be sorted by project.
func aggregateByProject(rows []containerStats) []projectStats {
	var result []projectStats
	for _, r := range rows {
		if len(result) == 0 || result[len(result)-1].name != r.project {
			result = append(result, projectStats{name: r.project})
		}
		p := &result[len(result)-1]
		p.containers++
		p.add(r.usage)
	}
	return result
}

func sampleStats(ctx context.Context, apiClient client.ContainerAPIClient, id string) (container.StatsResponse, error) {
	// a single sample is enough as long as the daemon provides the previous one to compute the CPU delta
	res, err := apiClient.ContainerStats(ctx, id, client.ContainerStatsOptions{IncludePreviousSample: true})
	if err != nil {
		return container.StatsResponse{}, err
	}
	defer res.Body.Close() //nolint:errcheck

	var v container.StatsResponse
	err = json.NewDecoder(res.Body).Decode(&v)
	return v, err
}

// containerUsage computes the same figures as `docker stats` for a Linux container. It follows the helpers of
// docker/cli (cli/command/container/stats_helpers.go: calculateCPUPercentUnix, calculateMemUsageUnixNoCache,
// calculateBlockIO and calculateNetwork), which must be kept in step with it.
func containerUsage(v container.StatsResponse) usage {
	u := usage{
		memory: float64(v.MemoryStats.Usage),
		pids:   v.PidsStats.Current,
	}

	// page cache is not memory the container needs: cgroup v1 reports it as total_inactive_file, cgroup v2 as inactive_file
	for _, key := range []string{"total_inactive_file", "inactive_file"} {
		if cache, ok := v.MemoryStats.Stats[key]; ok && cache < v.MemoryStats.Usage {
			u.memory = float64(v.MemoryStats.Usage - cache)
			break
		}
	}

	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
	onlineCPUs := float64(v.CPUStats.OnlineCPUs)
	if onlineCPUs == 0 {
		onlineCPUs = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
	}
	if cpuDelta > 0 && systemDelta > 0 {
		u.cpuPercent = cpuDelta / systemDelta * onlineCPUs * 100
	}

	for _, n := range v.Networks {
		u.netRx += float64(n.RxBytes)
		u.netTx += float64(n.TxBytes)
	}
	for _, e := range v.BlkioStats.IoServiceBytesRecursive {
		switch {
		case strings.EqualFold(e.Op, "read"):
			u.blockRead += float64(e.Value)
		case strings.EqualFold(e.Op, "write"):
			u.blockWrite += float64(e.Value)
		}
	}
	return u
}

func printContainerStats(out io.Writer, format string, rows []containerStats) error {
	view := make([]containerStatsView, len(rows))
	for i, r := range rows {
		memPerc := 0.0
		if r.memLimit > 0 {
			memPerc = r.memory / r.memLimit * 100
		}
		view[i] = containerStatsView{
			Project:  r.project,
			Name:     r.name,
			CPUPerc:  fmt.Sprintf("%.2f%%", r.cpuPercent),
			MemUsage: units.BytesSize(r.memory) + " / " + units.BytesSize(r.memLimit),
			MemPerc:  fmt.Sprintf("%.2f%%", memPerc),
			NetIO:    ioString(r.netRx, r.netTx),
			BlockIO:  ioString(r.blockRead, r.blockWrite),
			PIDs:     r.pids,
		}
	}
	return formatter.Print(view, statsFormat(format), out, func(w io.Writer) {
		for _, v := range view {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\n", v.Project, v.Name, v.CPUPerc, v.MemUsage, v.MemPerc, v.NetIO, v.BlockIO, v.PIDs)
		}
	}, "PROJECT", "NAME", "CPU %", "MEM USAGE / LIMIT", "MEM %", "NET I/O", "BLOCK I/O", "PIDS")
}

func printProjectStats(out io.Writer, format string, stats []projectStats) error {
	view := make([]projectStatsView, len(stats))
	for i, s := range stats {
		view[i] = projectStatsView{
			Project:    s.name,
			Containers: s.containers,
			CPUPerc:    fmt.Sprintf("%.2f%%", s.cpuPercent),
			MemUsage:   units.BytesSize(s.memory),
			NetIO:      ioString(s.netRx, s.netTx),
			BlockIO:    ioString(s.blockRead, s.blockWrite),
			PIDs:       s.pids,
		}
	}
	return formatter.Print(view, statsFormat(format), out, func(w io.Writer) {
		for _, v := range view {
			_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%d\n", v.Project, v.Containers, v.CPUPerc, v.MemUsage, v.NetIO, v.BlockIO, v.PIDs)
		}
	}, "PROJECT", "CONTAINERS", "CPU %", "MEM USAGE", "NET I/O", "BLOCK I/O", "PIDS")
}

// statsFormat makes the json format print one object per line, like `docker stats`, instead of an array.
func statsFormat(format string) string {
	if strings.EqualFold(format, formatter.JSON) {
		return formatter.TemplateLegacyJSON
	}
	return format
}

func ioString(in, out float64) string {
	return units.HumanSizeWithPrecision(in, 3) + " / " + units.HumanSizeWithPrecision(out, 3)
}
