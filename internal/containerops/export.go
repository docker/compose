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

package containerops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/client"
	"github.com/moby/sys/atomicwriter"

	"github.com/docker/compose/v5/internal/engine"
	"github.com/docker/compose/v5/internal/naming"
	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/api"
)

// Export writes the filesystem of a container of a service as a tar archive.
func (s *Service) Export(ctx context.Context, projectName string, options api.ExportOptions) error {
	projectName = strings.ToLower(projectName)

	container, err := s.backend.SpecifiedContainer(ctx, projectName, engine.OneOffInclude, false, options.Service, options.Index)
	if err != nil {
		return err
	}

	if options.Output == "" {
		if s.backend.CLI().Out().IsTerminal() {
			return errors.New("output option is required when exporting to terminal")
		}
	} else if err := command.ValidateOutputPath(options.Output); err != nil {
		return fmt.Errorf("failed to export container: %w", err)
	}

	name := naming.CanonicalContainerName(container)
	s.backend.Events().On(api.Resource{
		ID:     name,
		Text:   api.StatusExporting,
		Status: api.Working,
	})

	responseBody, err := s.backend.APIClient().ContainerExport(ctx, container.ID, client.ContainerExportOptions{})
	if err != nil {
		return err
	}

	defer func() {
		if err := responseBody.Close(); err != nil {
			s.backend.Events().On(progress.Error(name, "Failed to close response body: "+err.Error()))
		}
	}()

	if !s.backend.DryRun() {
		if options.Output == "" {
			_, err := io.Copy(s.backend.CLI().Out(), responseBody)
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

	s.backend.Events().On(api.Resource{
		ID:     name,
		Text:   api.StatusExported,
		Status: api.Done,
	})

	return nil
}
