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
	"net/netip"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/naming"
	compose "github.com/docker/compose/v5/pkg/api"
)

func TestPorts(t *testing.T) {
	const service = "web"

	ports := []container.PortSummary{
		{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		{IP: netip.MustParseAddr("::"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		{PrivatePort: 53, PublicPort: 5353, Type: "udp"},
		{PrivatePort: 3000, Type: "tcp"}, // exposed but not published: no host IP/port
	}

	tests := []struct {
		name     string
		port     uint16
		protocol string
		want     compose.PortPublishers
		wantErr  string
	}{
		{
			name: "no port and no protocol lists every mapping",
			want: compose.PortPublishers{
				{TargetPort: 53, PublishedPort: 5353, Protocol: "udp"},
				{URL: "0.0.0.0", TargetPort: 80, PublishedPort: 8080, Protocol: "tcp"},
				{URL: "::", TargetPort: 80, PublishedPort: 8080, Protocol: "tcp"},
				{TargetPort: 3000, Protocol: "tcp"},
			},
		},
		{
			name:     "protocol filters the list without a port",
			protocol: "udp",
			want: compose.PortPublishers{
				{TargetPort: 53, PublishedPort: 5353, Protocol: "udp"},
			},
		},
		{
			name:     "port and protocol match every dual-stack mapping for that port",
			port:     80,
			protocol: "tcp",
			want: compose.PortPublishers{
				{URL: "0.0.0.0", TargetPort: 80, PublishedPort: 8080, Protocol: "tcp"},
				{URL: "::", TargetPort: 80, PublishedPort: 8080, Protocol: "tcp"},
			},
		},
		{
			name:     "unmatched port returns an error naming the container",
			port:     9999,
			protocol: "tcp",
			wantErr:  "no port 9999/tcp for container 123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			api, cli := prepareMocks(mockCtrl)
			tested, err := NewComposeService(cli)
			assert.NilError(t, err)

			projectName := strings.ToLower(testProject)
			ctr := testContainer(service, "123", false)
			ctr.Ports = append([]container.PortSummary(nil), ports...)

			api.EXPECT().ContainerList(t.Context(), client.ContainerListOptions{
				Filters: naming.ProjectFilter(projectName).Add("label", naming.ServiceFilter(service), compose.ConfigHashLabel),
			}).Return(client.ContainerListResult{Items: []container.Summary{ctr}}, nil)

			got, err := tested.Ports(t.Context(), projectName, service, tt.port, compose.PortOptions{Protocol: tt.protocol})
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, got, tt.want)
		})
	}
}
