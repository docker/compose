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
	"fmt"
	"io"
	"strings"

	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
)

func (s *composeService) Export(ctx context.Context, projectName string, options api.ExportOptions) error {
	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		return s.export(ctx, projectName, options)
	}, s.stdinfo(), "Exporting")
}

func (s *composeService) export(ctx context.Context, projectName string, options api.ExportOptions) error {
	projectName = strings.ToLower(projectName)

	container, err := s.getSpecifiedContainer(ctx, projectName, oneOffInclude, false, options.Service, options.Index)
	if err != nil {
		return err
	}

	if options.Output == "" && s.dockerCli.Out().IsTerminal() {
		return fmt.Errorf("output option is required when exporting to terminal")
	}

	if err := command.ValidateOutputPath(options.Output); err != nil {
		return fmt.Errorf("failed to export container: %w", err)
	}

	clnt := s.dockerCli.Client()

	w := progress.ContextWriter(ctx)

	name := getCanonicalContainerName(container)
	msg := fmt.Sprintf("export %s to %s", name, options.Output)

	w.Event(progress.Event{
		ID:         name,
		Text:       msg,
		Status:     progress.Working,
		StatusText: "Exporting",
	})

	responseBody, err := clnt.ContainerExport(ctx, container.ID)
	if err != nil {
		return err
	}

	defer func() {
		if err := responseBody.Close(); err != nil {
			w.Event(progress.Event{
				ID:         name,
				Text:       msg,
				Status:     progress.Error,
				StatusText: fmt.Sprintf("Failed to close response body: %v", err),
			})
		}
	}()

	if !s.dryRun {
		if options.Output == "" {
			_, err := io.Copy(s.dockerCli.Out(), responseBody)
			return err
		}

		if err := command.CopyToFile(options.Output, responseBody); err != nil {
			return err
		}
	}

	w.Event(progress.Event{
		ID:         name,
		Text:       msg,
		Status:     progress.Done,
		StatusText: "Exported",
	})

	return nil
}
