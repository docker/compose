from __future__ import absolute_import
from __future__ import unicode_literals

import logging

from docker.errors import NotFound
from docker.utils import create_ipam_config
from docker.utils import create_ipam_pool

from .config import ConfigurationError


log = logging.getLogger(__name__)


class Network(object):
    def __init__(self, client, project, name, driver=None, driver_opts=None,
                 ipam=None, external_name=None, internal=False, enable_ipv6=False,
                 labels=None):
        self.client = client
        self.project = project
        self.name = name
        self.driver = driver
        self.driver_opts = driver_opts
        self.ipam = create_ipam_config_from_dict(ipam)
        self.external_name = external_name
        self.internal = internal
        self.enable_ipv6 = enable_ipv6
        self.labels = labels

    def ensure(self):
        if self.external_name:
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
                        name=self.external_name,
                        command='docker network create'
                    )
                )
            return

        try:
            data = self.inspect()
            if self.driver and data['Driver'] != self.driver:
                raise ConfigurationError(
                    'Network "{}" needs to be recreated - driver has changed'
                    .format(self.full_name))
            if data['Options'] != (self.driver_opts or {}):
                raise ConfigurationError(
                    'Network "{}" needs to be recreated - options have changed'
                    .format(self.full_name))
        except NotFound:
            driver_name = 'the default driver'
            if self.driver:
                driver_name = 'driver "{}"'.format(self.driver)

            log.info(
                'Creating network "{}" with {}'
                .format(self.full_name, driver_name)
            )

            self.client.create_network(
                name=self.full_name,
                driver=self.driver,
                options=self.driver_opts,
                ipam=self.ipam,
                internal=self.internal,
                enable_ipv6=self.enable_ipv6,
                labels=self.labels,
            )

    def remove(self):
        if self.external_name:
            log.info("Network %s is external, skipping", self.full_name)
            return

        log.info("Removing network {}".format(self.full_name))
        self.client.remove_network(self.full_name)

    def inspect(self):
        return self.client.inspect_network(self.full_name)

    @property
    def full_name(self):
        if self.external_name:
            return self.external_name
        return '{0}_{1}'.format(self.project, self.name)


def create_ipam_config_from_dict(ipam_dict):
    if not ipam_dict:
        return None

    return create_ipam_config(
        driver=ipam_dict.get('driver'),
        pool_configs=[
            create_ipam_pool(
                subnet=config.get('subnet'),
                iprange=config.get('ip_range'),
                gateway=config.get('gateway'),
                aux_addresses=config.get('aux_addresses'),
            )
            for config in ipam_dict.get('config', [])
        ],
    )


def build_networks(name, config_data, client):
    network_config = config_data.networks or {}
    networks = {
        network_name: Network(
            client=client, project=name, name=network_name,
            driver=data.get('driver'),
            driver_opts=data.get('driver_opts'),
            ipam=data.get('ipam'),
            external_name=data.get('external_name'),
            internal=data.get('internal'),
            enable_ipv6=data.get('enable_ipv6'),
            labels=data.get('labels'),
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
                log.warn("Network %s not found.", network.full_name)

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
            networks[network.full_name] = netdef
        else:
            raise ConfigurationError(
                'Service "{}" uses an undefined network "{}"'
                .format(service_dict['name'], name))

    return networks
