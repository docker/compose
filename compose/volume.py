from __future__ import unicode_literals


class Volume(object):
    def __init__(self, client, project, name, driver=None, driver_opts=None):
        self.client = client
        self.project = project
        self.name = name
        self.driver = driver
        self.driver_opts = driver_opts

    def create(self):
        return self.client.create_volume(self.name, self.driver, self.driver_opts)

    def remove(self):
        return self.client.remove_volume(self.name)

    def inspect(self):
        return self.client.inspect_volume(self.name)
