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
	"os"
	"path/filepath"

	_ "github.com/docker/buildx/driver/docker"           //nolint:blank-imports
	_ "github.com/docker/buildx/driver/docker-container" //nolint:blank-imports
	_ "github.com/docker/buildx/driver/kubernetes"       //nolint:blank-imports
	_ "github.com/docker/buildx/driver/remote"           //nolint:blank-imports
	"github.com/moby/buildkit/client"

	"github.com/docker/buildx/build"
	"github.com/docker/buildx/builder"
	"github.com/docker/buildx/util/dockerutil"
	xprogress "github.com/docker/buildx/util/progress"
	"github.com/docker/compose/v2/pkg/progress"
)

func (s *composeService) doBuildBuildkit(ctx context.Context, opts map[string]build.Options, mode string) (map[string]string, error) {
	b, err := builder.New(s.dockerCli)
	if err != nil {
		return nil, err
	}

	nodes, err := b.LoadNodes(ctx, false)
	if err != nil {
		return nil, err
	}

	var response map[string]*client.SolveResponse
	if s.dryRun {
		response = s.dryRunBuildResponse(ctx, opts)
	} else {
		// Progress needs its own context that lives longer than the
		// build one otherwise it won't read all the messages from
		// build and will lock
		progressCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		w, err := xprogress.NewPrinter(progressCtx, s.stdout(), os.Stdout, mode)
		if err != nil {
			return nil, err
		}
		response, err = build.Build(ctx, nodes, opts, dockerutil.NewClient(s.dockerCli), filepath.Dir(s.configFile().Filename), w)
		errW := w.Wait()
		if err == nil {
			err = errW
		}
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

	return imagesBuilt, err
}

func (s composeService) dryRunBuildResponse(ctx context.Context, options map[string]build.Options) map[string]*client.SolveResponse {
	w := progress.ContextWriter(ctx)
	buildResponse := map[string]*client.SolveResponse{}
	for name, option := range options {
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
			Text:   fmt.Sprintf(`naming to %s`, option.Tags[0]),
		})
		buildResponse[name] = &client.SolveResponse{ExporterResponse: map[string]string{
			"containerimage.digest": dryRunUUID,
		}}
	}
	return buildResponse
}
