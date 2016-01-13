from __future__ import absolute_import
from __future__ import unicode_literals

import logging

from docker.errors import NotFound

from .config import ConfigurationError


log = logging.getLogger(__name__)


# TODO: support external networks
class Network(object):
    def __init__(self, client, project, name, driver=None, driver_opts=None):
        self.client = client
        self.project = project
        self.name = name
        self.driver = driver
        self.driver_opts = driver_opts

    def ensure(self):
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
        # TODO: don't remove external networks
        log.info("Removing network {}".format(self.full_name))
        self.client.remove_network(self.full_name)

    def inspect(self):
        return self.client.inspect_network(self.full_name)

    @property
    def full_name(self):
        return '{0}_{1}'.format(self.project, self.name)
