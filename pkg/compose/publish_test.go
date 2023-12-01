/*
   Copyright 2023 Docker Compose CLI authors

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
	"testing"

	"github.com/distribution/reference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInferOCIVersion(t *testing.T) {
	tests := []struct {
		ref  string
		want ociCompatibilityMode
	}{
		{
			ref:  "175142243308.dkr.ecr.us-east-1.amazonaws.com/compose:test",
			want: ociCompatibility1_0,
		},
		{
			ref:  "my-image:latest",
			want: ociCompatibility1_1,
		},
		{
			ref:  "docker.io/docker/compose:test",
			want: ociCompatibility1_1,
		},
		{
			ref:  "ghcr.io/docker/compose:test",
			want: ociCompatibility1_1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			named, err := reference.ParseDockerRef(tt.ref)
			require.NoErrorf(t, err, "Test issue - invalid ref: %s", tt.ref)
			assert.Equalf(t, tt.want, inferOCIVersion(named), "inferOCIVersion(%s)", tt.ref)
		})
	}
}
