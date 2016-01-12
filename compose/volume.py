from __future__ import absolute_import
from __future__ import unicode_literals

import logging

from docker.errors import NotFound


log = logging.getLogger(__name__)


class Volume(object):
    def __init__(self, client, project, name, driver=None, driver_opts=None,
                 external_name=None):
        self.client = client
        self.project = project
        self.name = name
        self.driver = driver
        self.driver_opts = driver_opts
        self.external_name = external_name

    def create(self):
        return self.client.create_volume(
            self.full_name, self.driver, self.driver_opts
        )

    def remove(self):
        if self.external:
            log.info("Volume %s is external, skipping", self.full_name)
            return
        log.info("Removing volume %s", self.full_name)
        return self.client.remove_volume(self.full_name)

    def inspect(self):
        return self.client.inspect_volume(self.full_name)

    def exists(self):
        try:
            self.inspect()
        except NotFound:
            return False
        return True

    @property
    def external(self):
        return bool(self.external_name)

    @property
    def full_name(self):
        if self.external_name:
            return self.external_name
        return '{0}_{1}'.format(self.project, self.name)
