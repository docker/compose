class Service(object):
    def __init__(self, client, image, command):
        self.client = client
        self.image = image
        self.command = command

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
