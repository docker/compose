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
	"strings"

	"github.com/moby/moby/client"
)

// engineLabelDesktopAddress is used to detect that Compose is running with a
// Docker Desktop context. When this label is present, the value is an endpoint
// address for an in-memory socket (AF_UNIX or named pipe).
const engineLabelDesktopAddress = "com.docker.desktop.address"

func (s *composeService) isDesktopIntegrationActive(ctx context.Context) (bool, error) {
	res, err := s.apiClient().Info(ctx, client.InfoOptions{})
	if err != nil {
		return false, err
	}
	for _, l := range res.Info.Labels {
		k, _, ok := strings.Cut(l, "=")
		if ok && k == engineLabelDesktopAddress {
			return true, nil
		}
	}
	return false, nil
}
