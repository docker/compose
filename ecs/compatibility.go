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

package ecs

import (
	"fmt"

	"github.com/compose-spec/compose-go/compatibility"
	"github.com/compose-spec/compose-go/errdefs"
	"github.com/compose-spec/compose-go/types"
	"github.com/sirupsen/logrus"
)

func (b *ecsAPIService) checkCompatibility(project *types.Project) error {
	var checker compatibility.Checker = &fargateCompatibilityChecker{
		compatibility.AllowList{
			Supported: compatibleComposeAttributes,
		},
	}
	compatibility.Check(project, checker)
	for _, err := range checker.Errors() {
		if errdefs.IsIncompatibleError(err) {
			return err
		}
		logrus.Warn(err.Error())
	}
	if !compatibility.IsCompatible(checker) {
		return fmt.Errorf("compose file is incompatible with Amazon ECS")
	}
	return nil
}

type fargateCompatibilityChecker struct {
	compatibility.AllowList
}

var compatibleComposeAttributes = []string{
	"services.command",
	"services.container_name",
	"services.cap_drop",
	"services.depends_on",
	"services.deploy",
	"services.deploy.placement",
	"services.deploy.placement.constraints",
	"services.deploy.replicas",
	"services.deploy.resources.limits",
	"services.deploy.resources.limits.cpus",
	"services.deploy.resources.limits.memory",
	"services.deploy.resources.reservations",
	"services.deploy.resources.reservations.cpus",
	"services.deploy.resources.reservations.memory",
	"services.deploy.resources.reservations.generic_resources",
	"services.deploy.resources.reservations.generic_resources.discrete_resource_spec",
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
	"volumes.name",
	"volumes.driver_opts",
	"networks.external",
	"networks.name",
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
