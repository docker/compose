from __future__ import absolute_import
from __future__ import unicode_literals

from docker.errors import NotFound


class Volume(object):
    def __init__(self, client, project, name, driver=None, driver_opts=None):
        self.client = client
        self.project = project
        self.name = name
        self.driver = driver
        self.driver_opts = driver_opts

    def create(self):
        return self.client.create_volume(
            self.full_name, self.driver, self.driver_opts
        )

    def remove(self):
        return self.client.remove_volume(self.full_name)

    def inspect(self):
        return self.client.inspect_volume(self.full_name)

    @property
    def is_user_created(self):
        try:
            self.client.inspect_volume(self.name)
        except NotFound:
            return False

        return True

    @property
    def full_name(self):
        return '{0}_{1}'.format(self.project, self.name)
