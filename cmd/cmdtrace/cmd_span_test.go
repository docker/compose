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

package cmdtrace

import (
	"reflect"
	"testing"

	flag "github.com/spf13/pflag"
)

func TestGetFlags(t *testing.T) {
	// Initialize flagSet with flags
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	var (
		detach  string
		timeout string
	)
	fs.StringVar(&detach, "detach", "d", "")
	fs.StringVar(&timeout, "timeout", "t", "")
	_ = fs.Set("detach", "detach")
	_ = fs.Set("timeout", "timeout")

	tests := []struct {
		name     string
		input    *flag.FlagSet
		expected []string
	}{
		{
			name:     "NoFlags",
			input:    flag.NewFlagSet("NoFlags", flag.ContinueOnError),
			expected: nil,
		},
		{
			name:     "Flags",
			input:    fs,
			expected: []string{"detach", "timeout"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := getFlags(test.input)
			if !reflect.DeepEqual(result, test.expected) {
				t.Errorf("Expected %v, but got %v", test.expected, result)
			}
		})
	}

}
