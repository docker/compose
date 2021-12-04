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
	"testing"

	"github.com/compose-spec/compose-go/types"
)

func Test_getImageName(t *testing.T) {
	type args struct {
		s types.ServiceConfig
		p types.Project
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "image not mentioned explicitly",
			args: args{
				s: types.ServiceConfig{
					Name: "s1",
				},
				p: types.Project{
					Name: "p1",
				},
			},
			want: "p1_s1",
		},
		{
			name: "another image not mentioned explicitly",
			args: args{
				s: types.ServiceConfig{
					Name: "frontend",
				},
				p: types.Project{
					Name: "devops",
				},
			},
			want: "devops_frontend",
		},
		{
			name: "image mentioned explicitly",
			args: args{
				s: types.ServiceConfig{
					Name:  "s1",
					Image: "myimage:mytag",
				},
				p: types.Project{
					Name: "p1",
				},
			},
			want: "myimage:mytag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getImageName(tt.args.s, tt.args.p); got != tt.want {
				t.Errorf("getImageName() = %v, want %v", got, tt.want)
			}
		})
	}
}
