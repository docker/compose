package compatibility

import (
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/ecs-plugin/pkg/compose"
)

type Warning string
type Warnings []string

type CompatibilityChecker interface {
	CheckService(service *types.ServiceConfig)
	CheckCapAdd(service *types.ServiceConfig)
	CheckDNS(service *types.ServiceConfig)
	CheckDNSOpts(service *types.ServiceConfig)
	CheckDNSSearch(service *types.ServiceConfig)
	CheckDomainName(service *types.ServiceConfig)
	CheckExtraHosts(service *types.ServiceConfig)
	CheckHostname(service *types.ServiceConfig)
	CheckIpc(service *types.ServiceConfig)
	CheckLabels(service *types.ServiceConfig)
	CheckLinks(service *types.ServiceConfig)
	CheckLogging(service *types.ServiceConfig)
	CheckMacAddress(service *types.ServiceConfig)
	CheckNetworkMode(service *types.ServiceConfig)
	CheckPid(service *types.ServiceConfig)
	CheckSysctls(service *types.ServiceConfig)
	CheckTmpfs(service *types.ServiceConfig)
	CheckUserNSMode(service *types.ServiceConfig)
	Errors() []error
}

// Check the compose model do not use unsupported features and inject sane defaults for ECS deployment
func Check(project *compose.Project) []error {
	c := FargateCompatibilityChecker{}
	for i, service := range project.Services {
		c.CheckService(&service)
		project.Services[i] = service
	}
	return c.errors
}
