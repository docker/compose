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

package cmd

import (
	"testing"

	"gotest.tools/assert"
)

type caze struct {
	Actual   []string
	Expected []string
}

func TestVersionFormat(t *testing.T) {
	jsonCases := []caze{
		{
			Actual:   fixedJSONArgs([]string{}),
			Expected: []string{},
		},
		{
			Actual: fixedJSONArgs([]string{
				"docker",
				"version",
			}),
			Expected: []string{
				"docker",
				"version",
			},
		},
		{
			Actual: fixedJSONArgs([]string{
				"docker",
				"version",
				"--format",
				"json",
			}),
			Expected: []string{
				"docker",
				"version",
				"--format",
				"{{json .}}",
			},
		},
		{
			Actual: fixedJSONArgs([]string{
				"docker",
				"version",
				"--format",
				"jSoN",
			}),
			Expected: []string{
				"docker",
				"version",
				"--format",
				"{{json .}}",
			},
		},
		{
			Actual: fixedJSONArgs([]string{
				"docker",
				"version",
				"--format",
				"json",
				"--kubeconfig",
				"myKubeConfig",
			}),
			Expected: []string{
				"docker",
				"version",
				"--format",
				"{{json .}}",
				"--kubeconfig",
				"myKubeConfig",
			},
		},
		{
			Actual: fixedJSONArgs([]string{
				"--format",
				"json",
			}),
			Expected: []string{
				"--format",
				"{{json .}}",
			},
		},
	}
	prettyCases := []caze{
		{
			Actual:   fixedPrettyArgs([]string{}),
			Expected: []string{},
		},
		{
			Actual: fixedPrettyArgs([]string{
				"docker",
				"version",
			}),
			Expected: []string{
				"docker",
				"version",
			},
		},
		{
			Actual: fixedPrettyArgs([]string{
				"docker",
				"version",
				"--format",
				"pretty",
			}),
			Expected: []string{
				"docker",
				"version",
			},
		},
		{
			Actual: fixedPrettyArgs([]string{
				"docker",
				"version",
				"--format",
				"pRettY",
			}),
			Expected: []string{
				"docker",
				"version",
			},
		},
		{
			Actual: fixedPrettyArgs([]string{
				"docker",
				"version",
				"--format",
				"",
			}),
			Expected: []string{
				"docker",
				"version",
			},
		},
		{
			Actual: fixedPrettyArgs([]string{
				"docker",
				"version",
				"--format",
				"pretty",
				"--kubeconfig",
				"myKubeConfig",
			}),
			Expected: []string{
				"docker",
				"version",
				"--kubeconfig",
				"myKubeConfig",
			},
		},
		{
			Actual: fixedPrettyArgs([]string{
				"--format",
				"pretty",
			}),
			Expected: []string{},
		},
	}

	t.Run("json", func(t *testing.T) {
		for _, c := range jsonCases {
			assert.DeepEqual(t, c.Actual, c.Expected)
		}
	})

	t.Run("pretty", func(t *testing.T) {
		for _, c := range prettyCases {
			assert.DeepEqual(t, c.Actual, c.Expected)
		}
	})
}
