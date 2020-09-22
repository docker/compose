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

package context

import (
	"os"

	"github.com/spf13/pflag"
)

// ContextFlags are the global CLI flags
// nolint stutter
type ContextFlags struct {
	Context string
}

// AddContextFlags adds persistent (global) flags
func (c *ContextFlags) AddContextFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&c.Context, "context", "c", os.Getenv("DOCKER_CONTEXT"), "context")
}
