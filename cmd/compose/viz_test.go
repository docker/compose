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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferredIndentationStr(t *testing.T) {
	type args struct {
		size     int
		useSpace bool
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "should return '\\t\\t'",
			args: args{
				size:     2,
				useSpace: false,
			},
			want:    "\t\t",
			wantErr: false,
		},
		{
			name: "should return '    '",
			args: args{
				size:     4,
				useSpace: true,
			},
			want:    "    ",
			wantErr: false,
		},
		{
			name: "should return ''",
			args: args{
				size:     0,
				useSpace: false,
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "should return ''",
			args: args{
				size:     0,
				useSpace: true,
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "should throw error because indentation size < 0",
			args: args{
				size:     -1,
				useSpace: false,
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := preferredIndentationStr(tt.args.size, tt.args.useSpace)
			if tt.wantErr {
				require.Errorf(t, err, "preferredIndentationStr(%v, %v)", tt.args.size, tt.args.useSpace)
			} else {
				require.NoError(t, err)
				assert.Equalf(t, tt.want, got, "preferredIndentationStr(%v, %v)", tt.args.size, tt.args.useSpace)
			}
		})
	}
}
