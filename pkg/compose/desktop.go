/*
   Copyright 2024 Docker Compose CLI authors

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

	"github.com/docker/compose/v2/internal/desktop"
	"github.com/docker/compose/v2/internal/experimental"
	"github.com/sirupsen/logrus"
)

func (s *composeService) SetDesktopClient(cli *desktop.Client) {
	s.desktopCli = cli
}

func (s *composeService) SetExperiments(experiments *experimental.State) {
	s.experiments = experiments
}

func (s *composeService) manageDesktopFileSharesEnabled(ctx context.Context) bool {
	if !s.isDesktopIntegrationActive() {
		return false
	}

	// synchronized file share support in Docker Desktop is dependent upon
	// a variety of factors (settings, OS, etc), which this endpoint abstracts
	fileSharesConfig, err := s.desktopCli.GetFileSharesConfig(ctx)
	if err != nil {
		logrus.Debugf("Failed to retrieve file shares config: %v", err)
		return false
	}
	return fileSharesConfig.Active && fileSharesConfig.Compose.ManageBindMounts
}
