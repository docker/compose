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

package ecs

import (
	"testing"
)

func TestGuessMachineType(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    string
		wantErr bool
	}{
		{
			name: "1-gpus",
			yaml: `
services:
    learning:
        image: tensorflow/tensorflow:latest-gpus
        deploy:
            resources:
                reservations:
                   generic_resources:
                     - discrete_resource_spec:
                         kind: gpus
                         value: 1
`,
			want:    "g4dn.xlarge",
			wantErr: false,
		},
		{
			name: "4-gpus",
			yaml: `
services:
    learning:
        image: tensorflow/tensorflow:latest-gpus
        deploy:
            resources:
                reservations:
                   generic_resources: 
                     - discrete_resource_spec:
                         kind: gpus
                         value: 4
`,
			want:    "g4dn.12xlarge",
			wantErr: false,
		},
		{
			name: "1-gpus, high-memory",
			yaml: `
services:
    learning:
        image: tensorflow/tensorflow:latest-gpus
        deploy:
            resources:
                reservations: 
                   memory: 300Gb
                   generic_resources: 
                     - discrete_resource_spec:
                         kind: gpus
                         value: 2
`,
			want:    "g4dn.metal",
			wantErr: false,
		},
		{
			name: "1-gpus, high-cpu",
			yaml: `
services:
    learning:
        image: tensorflow/tensorflow:latest-gpus
        deploy:
            resources:
                reservations: 
                   memory: 32Gb
                   cpus: "32"
                   generic_resources: 
                     - discrete_resource_spec:
                         kind: gpus
                         value: 2
`,
			want:    "g4dn.12xlarge",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := loadConfig(t, tt.yaml)
			got, err := guessMachineType(project)
			if (err != nil) != tt.wantErr {
				t.Errorf("guessMachineType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("guessMachineType() got = %v, want %v", got, tt.want)
			}
		})
	}
}
