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

package e2e

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestComposePull(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("Verify image pulled", func(t *testing.T) {
		// cleanup existing images
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/compose-pull/simple", "down", "--rmi", "all")

		res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/compose-pull/simple", "pull")
		output := res.Combined()

		assert.Assert(t, strings.Contains(output, "simple Pulled"))
		assert.Assert(t, strings.Contains(output, "another Pulled"))

		// verify default policy is 'always' for pull command
		res = c.RunDockerComposeCmd(t, "--project-directory", "fixtures/compose-pull/simple", "pull")
		output = res.Combined()

		assert.Assert(t, strings.Contains(output, "simple Pulled"))
		assert.Assert(t, strings.Contains(output, "another Pulled"))
	})

	t.Run("Verify a image is pulled once", func(t *testing.T) {
		// cleanup existing images
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/compose-pull/duplicate-images", "down", "--rmi", "all")

		res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/compose-pull/duplicate-images", "pull")
		output := res.Combined()

		if strings.Contains(output, "another Pulled") {
			assert.Assert(t, strings.Contains(output, "another Pulled"))
			assert.Assert(t, strings.Contains(output, "Skipped - Image is already being pulled by another"))
		} else {
			assert.Assert(t, strings.Contains(output, "simple Pulled"))
			assert.Assert(t, strings.Contains(output, "Skipped - Image is already being pulled by simple"))
		}
	})

	t.Run("Verify skipped pull if image is already present locally", func(t *testing.T) {
		// make sure the required image is present
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/compose-pull/image-present-locally", "pull")

		res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/compose-pull/image-present-locally", "pull")
		output := res.Combined()

		assert.Assert(t, strings.Contains(output, "simple Skipped - Image is already present locally"))
		// image with :latest tag gets pulled regardless if pull_policy: missing or if_not_present
		assert.Assert(t, strings.Contains(output, "latest Pulled"))
	})

	t.Run("Verify skipped no image to be pulled", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/compose-pull/no-image-name-given", "pull")
		output := res.Combined()

		assert.Assert(t, strings.Contains(output, "Skipped - No image to be pulled"))
	})
}
