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
	"github.com/docker/cli/cli/command"
)

// dockerCliContextInfo implements api.ContextInfo using Docker CLI
type dockerCliContextInfo struct {
	cli command.Cli
}

func (c *dockerCliContextInfo) CurrentContext() string {
	return c.cli.CurrentContext()
}

func (c *dockerCliContextInfo) ServerOSType() string {
	return c.cli.ServerInfo().OSType
}

func (c *dockerCliContextInfo) BuildKitEnabled() (bool, error) {
	return c.cli.BuildKitEnabled()
}
