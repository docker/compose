from __future__ import absolute_import
from __future__ import unicode_literals

import logging

from docker.errors import NotFound

from .config import ConfigurationError


log = logging.getLogger(__name__)


class Network(object):
    def __init__(self, client, project, name, driver=None, driver_opts=None,
                 external_name=None):
        self.client = client
        self.project = project
        self.name = name
        self.driver = driver
        self.driver_opts = driver_opts
        self.external_name = external_name

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
                    'Network {} needs to be recreated - driver has changed'
                    .format(self.full_name))
            if data['Options'] != (self.driver_opts or {}):
                raise ConfigurationError(
                    'Network {} needs to be recreated - options have changed'
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
                self.full_name, self.driver, self.driver_opts
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
