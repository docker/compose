/*
   Copyright 2020 Docker, Inc.

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

package ecs

import (
	"github.com/compose-spec/compose-go/compatibility"
	"github.com/compose-spec/compose-go/types"
)

type fargateCompatibilityChecker struct {
	compatibility.AllowList
}

var compatibleComposeAttributes = []string{
	"services.command",
	"services.container_name",
	"services.cap_drop",
	"services.depends_on",
	"services.deploy",
	"services.deploy.replicas",
	"services.deploy.resources.limits",
	"services.deploy.resources.limits.cpus",
	"services.deploy.resources.limits.memory",
	"services.deploy.resources.reservations",
	"services.deploy.resources.reservations.cpus",
	"services.deploy.resources.reservations.memory",
	"services.deploy.update_config",
	"services.deploy.update_config.parallelism",
	"services.entrypoint",
	"services.environment",
	"services.env_file",
	"services.healthcheck",
	"services.healthcheck.interval",
	"services.healthcheck.retries",
	"services.healthcheck.start_period",
	"services.healthcheck.test",
	"services.healthcheck.timeout",
	"services.image",
	"services.init",
	"services.logging",
	"services.logging.options",
	"services.networks",
	"services.ports",
	"services.ports.mode",
	"services.ports.target",
	"services.ports.protocol",
	"services.secrets",
	"services.secrets.source",
	"services.secrets.target",
	"services.user",
	"services.volumes",
	"services.volumes.read_only",
	"services.volumes.source",
	"services.volumes.target",
	"services.working_dir",
	"secrets.external",
	"secrets.name",
	"secrets.file",
	"volumes",
	"volumes.external",
}

func (c *fargateCompatibilityChecker) CheckImage(service *types.ServiceConfig) {
	if service.Image == "" {
		c.Incompatible("service %s doesn't define a Docker image to run", service.Name)
	}
}

func (c *fargateCompatibilityChecker) CheckPortsPublished(p *types.ServicePortConfig) {
	if p.Published == 0 {
		p.Published = p.Target
	}
	if p.Published != p.Target {
		c.Incompatible("published port can't be set to a distinct value than container port")
	}
}

func (c *fargateCompatibilityChecker) CheckCapAdd(service *types.ServiceConfig) {
	add := []string{}
	for _, cap := range service.CapAdd {
		switch cap {
		case "SYS_PTRACE":
			add = append(add, cap)
		default:
			c.Incompatible("ECS doesn't allow to add capability %s", cap)
		}
	}
	service.CapAdd = add
}

func (c *fargateCompatibilityChecker) CheckLoggingDriver(config *types.LoggingConfig) {
	if config.Driver != "" && config.Driver != "awslogs" {
		c.Unsupported("services.logging.driver %s is not supported", config.Driver)
	}
}

func (c *fargateCompatibilityChecker) CheckVolumeConfigExternal(config *types.VolumeConfig) {
	if !config.External.External {
		c.Unsupported("non-external volumes are not supported")
	}
}
