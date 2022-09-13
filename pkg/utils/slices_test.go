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

package utils

import (
	"testing"

	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestContains(t *testing.T) {
	source := []specs.Platform{
		{
			Architecture: "linux/amd64",
			OS:           "darwin",
			OSVersion:    "",
			OSFeatures:   nil,
			Variant:      "",
		},
		{
			Architecture: "linux/arm64",
			OS:           "linux",
			OSVersion:    "12",
			OSFeatures:   nil,
			Variant:      "v8",
		},
		{
			Architecture: "",
			OS:           "",
			OSVersion:    "",
			OSFeatures:   nil,
			Variant:      "",
		},
	}

	type args struct {
		origin  []specs.Platform
		element specs.Platform
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "element found",
			args: args{
				origin: source,
				element: specs.Platform{
					Architecture: "linux/arm64",
					OS:           "linux",
					OSVersion:    "12",
					OSFeatures:   nil,
					Variant:      "v8",
				},
			},
			want: true,
		},
		{
			name: "element not found",
			args: args{
				origin: source,
				element: specs.Platform{
					Architecture: "linux/arm64",
					OS:           "darwin",
					OSVersion:    "12",
					OSFeatures:   nil,
					Variant:      "v8",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Contains(tt.args.origin, tt.args.element); got != tt.want {
				t.Errorf("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}
