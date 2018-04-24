from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import re
from collections import OrderedDict

from docker.errors import NotFound
from docker.types import IPAMConfig
from docker.types import IPAMPool
from docker.utils import version_gte
from docker.utils import version_lt

from . import __version__
from .config import ConfigurationError
from .const import LABEL_NETWORK
from .const import LABEL_PROJECT
from .const import LABEL_VERSION


log = logging.getLogger(__name__)

OPTS_EXCEPTIONS = [
    'com.docker.network.driver.overlay.vxlanid_list',
    'com.docker.network.windowsshim.hnsid',
    'com.docker.network.windowsshim.networkname'
]


class Network(object):
    def __init__(self, client, project, name, driver=None, driver_opts=None,
                 ipam=None, external=False, internal=False, enable_ipv6=False,
                 labels=None, custom_name=False):
        self.client = client
        self.project = project
        self.name = name
        self.driver = driver
        self.driver_opts = driver_opts
        self.ipam = create_ipam_config_from_dict(ipam)
        self.external = external
        self.internal = internal
        self.enable_ipv6 = enable_ipv6
        self.labels = labels
        self.custom_name = custom_name
        self.legacy = None

    def ensure(self):
        if self.external:
            if self.driver == 'overlay':
                # Swarm nodes do not register overlay networks that were
                # created on a different node unless they're in use.
                # See docker/compose#4399
                return
            try:
                self.inspect()
                log.debug(
                    'Network {0} declared as external. No new '
                    'network will be created.'.format(self.name)
                )
            except NotFound:
                raise ConfigurationError(
                    'Network {name} declared as external, but could'
                    ' not be found. Please create the network manually'
                    ' using `{command} {name}` and try again.'.format(
                        name=self.full_name,
                        command='docker network create'
                    )
                )
            return

        self._set_legacy_flag()
        try:
            data = self.inspect(legacy=self.legacy)
            check_remote_network_config(data, self)
        except NotFound:
            driver_name = 'the default driver'
            if self.driver:
                driver_name = 'driver "{}"'.format(self.driver)

            log.info(
                'Creating network "{}" with {}'.format(self.full_name, driver_name)
            )

            self.client.create_network(
                name=self.full_name,
                driver=self.driver,
                options=self.driver_opts,
                ipam=self.ipam,
                internal=self.internal,
                enable_ipv6=self.enable_ipv6,
                labels=self._labels,
                attachable=version_gte(self.client._version, '1.24') or None,
                check_duplicate=True,
            )

    def remove(self):
        if self.external:
            log.info("Network %s is external, skipping", self.true_name)
            return

        log.info("Removing network {}".format(self.true_name))
        self.client.remove_network(self.true_name)

    def inspect(self, legacy=False):
        if legacy:
            return self.client.inspect_network(self.legacy_full_name)
        return self.client.inspect_network(self.full_name)

    @property
    def legacy_full_name(self):
        if self.custom_name:
            return self.name
        return '{0}_{1}'.format(
            re.sub(r'[_-]', '', self.project), self.name
        )

    @property
    def full_name(self):
        if self.custom_name:
            return self.name
        return '{0}_{1}'.format(self.project, self.name)

    @property
    def true_name(self):
        self._set_legacy_flag()
        if self.legacy:
            return self.legacy_full_name
        return self.full_name

    @property
    def _labels(self):
        if version_lt(self.client._version, '1.23'):
            return None
        labels = self.labels.copy() if self.labels else {}
        labels.update({
            LABEL_PROJECT: self.project,
            LABEL_NETWORK: self.name,
            LABEL_VERSION: __version__,
        })
        return labels

    def _set_legacy_flag(self):
        if self.legacy is not None:
            return
        try:
            data = self.inspect(legacy=True)
            self.legacy = data is not None
        except NotFound:
            self.legacy = False


def create_ipam_config_from_dict(ipam_dict):
    if not ipam_dict:
        return None

    return IPAMConfig(
        driver=ipam_dict.get('driver') or 'default',
        pool_configs=[
            IPAMPool(
                subnet=config.get('subnet'),
                iprange=config.get('ip_range'),
                gateway=config.get('gateway'),
                aux_addresses=config.get('aux_addresses'),
            )
            for config in ipam_dict.get('config', [])
        ],
        options=ipam_dict.get('options')
    )


class NetworkConfigChangedError(ConfigurationError):
    def __init__(self, net_name, property_name):
        super(NetworkConfigChangedError, self).__init__(
            'Network "{}" needs to be recreated - {} has changed'.format(
                net_name, property_name
            )
        )


def check_remote_ipam_config(remote, local):
    remote_ipam = remote.get('IPAM')
    ipam_dict = create_ipam_config_from_dict(local.ipam)
    if local.ipam.get('driver') and local.ipam.get('driver') != remote_ipam.get('Driver'):
        raise NetworkConfigChangedError(local.true_name, 'IPAM driver')
    if len(ipam_dict['Config']) != 0:
        if len(ipam_dict['Config']) != len(remote_ipam['Config']):
            raise NetworkConfigChangedError(local.true_name, 'IPAM configs')
        remote_configs = sorted(remote_ipam['Config'], key='Subnet')
        local_configs = sorted(ipam_dict['Config'], key='Subnet')
        while local_configs:
            lc = local_configs.pop()
            rc = remote_configs.pop()
            if lc.get('Subnet') != rc.get('Subnet'):
                raise NetworkConfigChangedError(local.true_name, 'IPAM config subnet')
            if lc.get('Gateway') is not None and lc.get('Gateway') != rc.get('Gateway'):
                raise NetworkConfigChangedError(local.true_name, 'IPAM config gateway')
            if lc.get('IPRange') != rc.get('IPRange'):
                raise NetworkConfigChangedError(local.true_name, 'IPAM config ip_range')
            if sorted(lc.get('AuxiliaryAddresses')) != sorted(rc.get('AuxiliaryAddresses')):
                raise NetworkConfigChangedError(local.true_name, 'IPAM config aux_addresses')

    remote_opts = remote_ipam.get('Options') or {}
    local_opts = local.ipam.get('Options') or {}
    for k in set.union(set(remote_opts.keys()), set(local_opts.keys())):
        if remote_opts.get(k) != local_opts.get(k):
            raise NetworkConfigChangedError(local.true_name, 'IPAM option "{}"'.format(k))


def check_remote_network_config(remote, local):
    if local.driver and remote.get('Driver') != local.driver:
        raise NetworkConfigChangedError(local.true_name, 'driver')
    local_opts = local.driver_opts or {}
    remote_opts = remote.get('Options') or {}
    for k in set.union(set(remote_opts.keys()), set(local_opts.keys())):
        if k in OPTS_EXCEPTIONS:
            continue
        if remote_opts.get(k) != local_opts.get(k):
            raise NetworkConfigChangedError(local.true_name, 'option "{}"'.format(k))

    if local.ipam is not None:
        check_remote_ipam_config(remote, local)

    if local.internal is not None and local.internal != remote.get('Internal', False):
        raise NetworkConfigChangedError(local.true_name, 'internal')
    if local.enable_ipv6 is not None and local.enable_ipv6 != remote.get('EnableIPv6', False):
        raise NetworkConfigChangedError(local.true_name, 'enable_ipv6')

    local_labels = local.labels or {}
    remote_labels = remote.get('Labels', {})
    for k in set.union(set(remote_labels.keys()), set(local_labels.keys())):
        if k.startswith('com.docker.'):  # We are only interested in user-specified labels
            continue
        if remote_labels.get(k) != local_labels.get(k):
            log.warn(
                'Network {}: label "{}" has changed. It may need to be'
                ' recreated.'.format(local.true_name, k)
            )


def build_networks(name, config_data, client):
    network_config = config_data.networks or {}
    networks = {
        network_name: Network(
            client=client, project=name,
            name=data.get('name', network_name),
            driver=data.get('driver'),
            driver_opts=data.get('driver_opts'),
            ipam=data.get('ipam'),
            external=bool(data.get('external', False)),
            internal=data.get('internal'),
            enable_ipv6=data.get('enable_ipv6'),
            labels=data.get('labels'),
            custom_name=data.get('name') is not None,
        )
        for network_name, data in network_config.items()
    }

    if 'default' not in networks:
        networks['default'] = Network(client, name, 'default')

    return networks


class ProjectNetworks(object):

    def __init__(self, networks, use_networking):
        self.networks = networks or {}
        self.use_networking = use_networking

    @classmethod
    def from_services(cls, services, networks, use_networking):
        service_networks = {
            network: networks.get(network)
            for service in services
            for network in get_network_names_for_service(service)
        }
        unused = set(networks) - set(service_networks) - {'default'}
        if unused:
            log.warn(
                "Some networks were defined but are not used by any service: "
                "{}".format(", ".join(unused)))
        return cls(service_networks, use_networking)

    def remove(self):
        if not self.use_networking:
            return
        for network in self.networks.values():
            try:
                network.remove()
            except NotFound:
                log.warn("Network %s not found.", network.true_name)

    def initialize(self):
        if not self.use_networking:
            return

        for network in self.networks.values():
            network.ensure()


def get_network_defs_for_service(service_dict):
    if 'network_mode' in service_dict:
        return {}
    networks = service_dict.get('networks', {'default': None})
    return dict(
        (net, (config or {}))
        for net, config in networks.items()
    )


def get_network_names_for_service(service_dict):
    return get_network_defs_for_service(service_dict).keys()


def get_networks(service_dict, network_definitions):
    networks = {}
    for name, netdef in get_network_defs_for_service(service_dict).items():
        network = network_definitions.get(name)
        if network:
            networks[network.true_name] = netdef
        else:
            raise ConfigurationError(
                'Service "{}" uses an undefined network "{}"'
                .format(service_dict['name'], name))

    return OrderedDict(sorted(
        networks.items(),
        key=lambda t: t[1].get('priority') or 0, reverse=True
    ))
