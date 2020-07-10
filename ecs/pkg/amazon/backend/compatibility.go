package backend

import (
	"github.com/compose-spec/compose-go/compatibility"
	"github.com/compose-spec/compose-go/types"
)

type FargateCompatibilityChecker struct {
	compatibility.AllowList
}

var compatibleComposeAttributes = []string{
	"services.command",
	"services.container_name",
	"services.cap_drop",
	"services.depends_on",
	"services.deploy",
	"services.deploy.replicas",
	"services.entrypoint",
	"services.environment",
	"service.image",
	"services.init",
	"services.healthcheck",
	"services.healthcheck.interval",
	"services.healthcheck.start_period",
	"services.healthcheck.test",
	"services.healthcheck.timeout",
	"services.networks",
	"services.ports",
	"services.ports.mode",
	"services.ports.target",
	"services.ports.protocol",
	"services.user",
	"services.working_dir",
}

func (c *FargateCompatibilityChecker) CheckImage(service *types.ServiceConfig) {
	if service.Image == "" {
		c.Incompatible("service %s doesn't define a Docker image to run", service.Name)
	}
}

func (c *FargateCompatibilityChecker) CheckPortsPublished(p *types.ServicePortConfig) {
	if p.Published == 0 {
		p.Published = p.Target
	}
	if p.Published != p.Target {
		c.Incompatible("published port can't be set to a distinct value than container port")
	}
}

func (c *FargateCompatibilityChecker) CheckCapAdd(service *types.ServiceConfig) {
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
