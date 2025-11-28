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
	"github.com/moby/moby/client"
	"github.com/moby/sys/atomicwriter"

	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Export(ctx context.Context, projectName string, options api.ExportOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.export(ctx, projectName, options)
	}, "export", s.events)
}

func (s *composeService) export(ctx context.Context, projectName string, options api.ExportOptions) error {
	projectName = strings.ToLower(projectName)

	container, err := s.getSpecifiedContainer(ctx, projectName, oneOffInclude, false, options.Service, options.Index)
	if err != nil {
		return err
	}

	if options.Output == "" {
		if s.stdout().IsTerminal() {
			return fmt.Errorf("output option is required when exporting to terminal")
		}
	} else if err := command.ValidateOutputPath(options.Output); err != nil {
		return fmt.Errorf("failed to export container: %w", err)
	}

	name := getCanonicalContainerName(container)
	s.events.On(api.Resource{
		ID:     name,
		Text:   api.StatusExporting,
		Status: api.Working,
	})

	responseBody, err := s.apiClient().ContainerExport(ctx, container.ID, client.ContainerExportOptions{})
	if err != nil {
		return err
	}

	defer func() {
		if err := responseBody.Close(); err != nil {
			s.events.On(errorEventf(name, "Failed to close response body: %s", err.Error()))
		}
	}()

	if !s.dryRun {
		if options.Output == "" {
			_, err := io.Copy(s.stdout(), responseBody)
			return err
		} else {
			writer, err := atomicwriter.New(options.Output, 0o600)
			if err != nil {
				return err
			}
			defer func() { _ = writer.Close() }()

			_, err = io.Copy(writer, responseBody)
			return err
		}
	}

	s.events.On(api.Resource{
		ID:     name,
		Text:   api.StatusExported,
		Status: api.Done,
	})

	return nil
}
