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
	"os"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/sirupsen/logrus"
)

// WithEnvFiles wraps cli.WithEnvFiles so a default .env that exists but
// cannot be stat'd (typically permission denied on the working directory)
// is skipped instead of failing the command. Explicit --env-file paths
// still error if they are unreadable.
func WithEnvFiles(files ...string) cli.ProjectOptionsFn {
	return func(o *cli.ProjectOptions) error {
		err := cli.WithEnvFiles(files...)(o)
		if err == nil || len(files) > 0 {
			return err
		}
		if os.IsPermission(err) {
			logrus.Warnf("ignoring unreadable default .env file: %v", err)
			return nil
		}
		return err
	}
}
