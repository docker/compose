import re


class Service(object):
    def __init__(self, name, client=None, image=None, command=None, links=None):
        if not re.match('^[a-zA-Z0-9_]+$', name):
            raise ValueError('Invalid name: %s' % name)

        self.name = name
        self.client = client
        self.image = image
        self.command = command
        self.links = links or []

    @property
    def containers(self):
        return self.client.containers()

    def start(self):
        if len(self.containers) == 0:
            self.start_container()

    def stop(self):
        self.scale(0)

    def scale(self, num):
        while len(self.containers) < num:
            self.start_container()

        while len(self.containers) > num:
            self.stop_container()

    def start_container(self):
        container = self.client.create_container(self.image, self.command)
        self.client.start(container['Id'])

    def stop_container(self):
        self.client.kill(self.containers[0]['Id'])
