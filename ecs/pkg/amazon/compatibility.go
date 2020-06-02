package amazon

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/compose-spec/compose-go/types"
)

type FargateCompatibilityChecker struct {
	errors []error
}

func (c *FargateCompatibilityChecker) error(message string, args ...interface{}) {
	c.errors = append(c.errors, fmt.Errorf(message, args...))
}

func (c *FargateCompatibilityChecker) Errors() []error {
	return c.errors
}

func (c *FargateCompatibilityChecker) CheckService(service *types.ServiceConfig) {
	c.CheckCapAdd(service)
	c.CheckDNS(service)
	c.CheckDNSOpts(service)
	c.CheckDNSSearch(service)
	c.CheckDomainName(service)
	c.CheckExtraHosts(service)
	c.CheckHostname(service)
	c.CheckIpc(service)
	c.CheckLabels(service)
	c.CheckLinks(service)
	c.CheckLogging(service)
	c.CheckMacAddress(service)
	c.CheckNetworkMode(service)
	c.CheckPid(service)
	c.CheckSysctls(service)
	c.CheckTmpfs(service)
	c.CheckUserNSMode(service)
}

func (c *FargateCompatibilityChecker) CheckNetworkMode(service *types.ServiceConfig) {
	if service.NetworkMode != "" && service.NetworkMode != ecs.NetworkModeAwsvpc {
		c.error("'network_mode' %q is not supported", service.NetworkMode)
	}
	service.NetworkMode = ecs.NetworkModeAwsvpc
}

func (c *FargateCompatibilityChecker) CheckLinks(service *types.ServiceConfig) {
	if len(service.Links) != 0 {
		c.error("'links' is not supported")
		service.Links = nil
	}
}

func (c *FargateCompatibilityChecker) CheckLogging(service *types.ServiceConfig) {
	c.CheckLoggingDriver(service)
}

func (c *FargateCompatibilityChecker) CheckLoggingDriver(service *types.ServiceConfig) {
	if service.LogDriver != "" && service.LogDriver != ecs.LogDriverAwslogs {
		c.error("'log_driver' %q is not supported", service.LogDriver)
		service.LogDriver = ecs.LogDriverAwslogs
	}
}

func (c *FargateCompatibilityChecker) CheckPid(service *types.ServiceConfig) {
	if service.Pid != "" {
		c.error("'pid' is not supported")
		service.Pid = ""
	}
}

func (c *FargateCompatibilityChecker) CheckUserNSMode(service *types.ServiceConfig) {
	if service.UserNSMode != "" {
		c.error("'userns_mode' is not supported")
		service.UserNSMode = ""
	}
}

func (c *FargateCompatibilityChecker) CheckIpc(service *types.ServiceConfig) {
	if service.Ipc != "" {
		c.error("'ipc' is not supported")
		service.Ipc = ""
	}
}

func (c *FargateCompatibilityChecker) CheckMacAddress(service *types.ServiceConfig) {
	if service.MacAddress != "" {
		c.error("'mac_address' is not supported")
		service.MacAddress = ""
	}
}

func (c *FargateCompatibilityChecker) CheckHostname(service *types.ServiceConfig) {
	if service.Hostname != "" {
		c.error("'hostname' is not supported")
		service.Hostname = ""
	}
}

func (c *FargateCompatibilityChecker) CheckDomainName(service *types.ServiceConfig) {
	if service.DomainName != "" {
		c.error("'domainname' is not supported")
		service.DomainName = ""
	}
}

func (c *FargateCompatibilityChecker) CheckDNSSearch(service *types.ServiceConfig) {
	if len(service.DNSSearch) > 0 {
		c.error("'dns_search' is not supported")
		service.DNSSearch = nil
	}
}

func (c *FargateCompatibilityChecker) CheckDNS(service *types.ServiceConfig) {
	if len(service.DNS) > 0 {
		c.error("'dns' is not supported")
		service.DNS = nil
	}
}

func (c *FargateCompatibilityChecker) CheckDNSOpts(service *types.ServiceConfig) {
	if len(service.DNSOpts) > 0 {
		c.error("'dns_opt' is not supported")
		service.DNSOpts = nil
	}
}

func (c *FargateCompatibilityChecker) CheckExtraHosts(service *types.ServiceConfig) {
	if len(service.ExtraHosts) > 0 {
		c.error("'extra_hosts' is not supported")
		service.ExtraHosts = nil
	}
}

func (c *FargateCompatibilityChecker) CheckCapAdd(service *types.ServiceConfig) {
	for i, v := range service.CapAdd {
		if v != "SYS_PTRACE" {
			c.error("'cap_add' %s is not supported", v)
			l := len(service.CapAdd)
			service.CapAdd[i] = service.CapAdd[l-1]
			service.CapAdd = service.CapAdd[:l-1]
		}
	}
}

func (c *FargateCompatibilityChecker) CheckTmpfs(service *types.ServiceConfig) {
	if len(service.Tmpfs) > 0 {
		c.error("'tmpfs' is not supported")
		service.Tmpfs = nil
	}
}

func (c *FargateCompatibilityChecker) CheckSysctls(service *types.ServiceConfig) {
	if len(service.Sysctls) > 0 {
		c.error("'sysctls' is not supported")
		service.Sysctls = nil
	}
}

func (c *FargateCompatibilityChecker) CheckLabels(service *types.ServiceConfig) {
	for k, v := range service.Labels {
		if v == "" {
			c.error("'labels' with an empty value is not supported")
			delete(service.Labels, k)
		}
	}
}

var _ CompatibilityChecker = &FargateCompatibilityChecker{}
