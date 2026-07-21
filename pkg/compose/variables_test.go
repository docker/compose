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

	"gotest.tools/v3/assert"
)

const variableMergeDomainName = "PIHOLE_DOMAIN"

func TestExtractVariablesKeepsRequiredOccurrence(t *testing.T) {
	variables := ExtractVariables(map[string]any{
		"services": map[string]any{
			"pihole": map[string]any{
				"labels": []any{
					"traefik.http.routers.pihole.rule=Host(`${PIHOLE_DOMAIN:?}`)",
					"traefik.http.middlewares.pihole-redirect.redirectregex.regex=^(https://${PIHOLE_DOMAIN})/?$",
				},
			},
		},
	})

	assert.Assert(t, variables[variableMergeDomainName].Required)
}
