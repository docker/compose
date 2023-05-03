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
	"crypto/sha1"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	_ "github.com/docker/buildx/driver/docker"           //nolint:blank-imports
	_ "github.com/docker/buildx/driver/docker-container" //nolint:blank-imports
	_ "github.com/docker/buildx/driver/kubernetes"       //nolint:blank-imports
	_ "github.com/docker/buildx/driver/remote"           //nolint:blank-imports
	"github.com/docker/compose/v2/pkg/progress"
	bclient "github.com/moby/buildkit/client"
	"github.com/opencontainers/go-digest"

	"github.com/docker/buildx/build"
	"github.com/docker/buildx/builder"
	"github.com/docker/buildx/util/dockerutil"
	xprogress "github.com/docker/buildx/util/progress"
)

type BuildOutput struct {
	service  string
	vertexes map[digest.Digest]*bclient.Vertex
	logs     map[digest.Digest]int
	w        progress.Writer
}

func sanitize(s string) string {
	split := strings.Split(strings.TrimSpace(s), "\n")
	return split[len(split)-1]
}

func (b *BuildOutput) Write(status *bclient.SolveStatus) {
	for _, v := range status.Vertexes {
		b.vertexes[v.Digest] = v

		txt := v.Name
		if v.Cached {
			txt = "CACHED " + v.Name
		}
		if v.ProgressGroup != nil {
			txt = txt + " => " + v.ProgressGroup.String()
		}
		status := progress.Working
		if v.Completed != nil {
			status = progress.Done
		}

		b.w.Event(progress.Event{
			ParentID:   b.service,
			ID:         v.Digest.String(),
			StatusText: sanitize(txt),
			Status:     status,
		})

	}

	for _, s := range status.Statuses {
		txt := s.Name
		if s.Current > 0 {
			txt = fmt.Sprintf("%s %dB", s.Name, s.Current)
		}
		b.w.Event(progress.Event{
			ParentID:   b.service,
			ID:         b.getVertexId(s.Vertex),
			StatusText: sanitize(txt),
			Status:     progress.Done,
		})
	}
	for _, s := range status.Warnings {
		b.w.Event(progress.Event{
			ParentID:   b.service,
			ID:         b.getVertexId(s.Vertex),
			StatusText: sanitize(string(s.Short)),
		})
	}
	for _, l := range status.Logs {
		var start int
		count := b.logs[l.Vertex]

		for idx, r := range l.Data {
			if r == '\n' || r == '\r' {
				line := string(l.Data[start:idx])
				start = idx + 1
				if len(line) == 0 {
					continue
				}
				line = strings.TrimRightFunc(line, unicode.IsSpace)
				b.w.Event(progress.Event{
					ParentID:   b.getVertexId(l.Vertex),
					ID:         fmt.Sprintf("#%d", count),
					StatusText: line,
					Status:     progress.Log,
				})
				count++
			}
		}
		b.logs[l.Vertex] = count
	}
}

func (b *BuildOutput) ValidateLogSource(digest digest.Digest, i interface{}) bool {
	return true
}

func (b BuildOutput) ClearLogSource(i interface{}) {

}

func (b *BuildOutput) getVertexId(vertex digest.Digest) string {
	return b.vertexes[vertex].Digest.String()
}

var _ xprogress.Writer = &BuildOutput{}

func (s *composeService) doBuildBuildkit(ctx context.Context, service string, opts build.Options, mode string) (map[string]string, error) {
	b, err := builder.New(s.dockerCli)
	if err != nil {
		return nil, err
	}

	nodes, err := b.LoadNodes(ctx, false)
	if err != nil {
		return nil, err
	}

	writer := progress.ContextWriter(ctx)
	writer.Event(progress.Event{ID: service, Status: progress.Working})
	var response map[string]*bclient.SolveResponse
	if s.dryRun {
		response = s.dryRunBuildResponse(ctx, service, opts)
	} else {
		w := BuildOutput{
			vertexes: map[digest.Digest]*bclient.Vertex{},
			logs:     map[digest.Digest]int{},
			service:  service,
			w:        writer,
		}

		response, err = build.Build(ctx, nodes, map[string]build.Options{service: opts}, dockerutil.NewClient(s.dockerCli), filepath.Dir(s.configFile().Filename), &w)
		if err != nil {
			return nil, WrapCategorisedComposeError(err, BuildFailure)
		}
	}

	imagesBuilt := map[string]string{}
	for name, img := range response {
		if img == nil || len(img.ExporterResponse) == 0 {
			continue
		}
		digest, ok := img.ExporterResponse["containerimage.digest"]
		if !ok {
			continue
		}
		imagesBuilt[name] = digest
	}

	if err != nil {
		writer.Event(progress.Event{ID: service, StatusText: err.Error(), Status: progress.Error})
	} else {
		writer.Event(progress.Event{ID: service, Status: progress.Done})
	}
	return imagesBuilt, err
}

func (s composeService) dryRunBuildResponse(ctx context.Context, name string, options build.Options) map[string]*bclient.SolveResponse {
	w := progress.ContextWriter(ctx)
	buildResponse := map[string]*bclient.SolveResponse{}
	dryRunUUID := fmt.Sprintf("dryRun-%x", sha1.Sum([]byte(name)))
	w.Event(progress.Event{
		ID:     " ",
		Status: progress.Done,
		Text:   fmt.Sprintf("build service %s", name),
	})
	w.Event(progress.Event{
		ID:     "==>",
		Status: progress.Done,
		Text:   fmt.Sprintf("==> writing image %s", dryRunUUID),
	})
	w.Event(progress.Event{
		ID:     "==> ==>",
		Status: progress.Done,
		Text:   fmt.Sprintf(`naming to %s`, options.Tags[0]),
	})
	buildResponse[name] = &bclient.SolveResponse{ExporterResponse: map[string]string{
		"containerimage.digest": dryRunUUID,
	}}
	return buildResponse
}
