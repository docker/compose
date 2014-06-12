from __future__ import unicode_literals
from __future__ import absolute_import
from fig.packages.docker import Client
from fig.service import Service
from fig.cli.utils import docker_url
from .. import unittest


class DockerClientTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.client = Client(docker_url())
        cls.client.pull('busybox', tag='latest')

    def setUp(self):
        for c in self.client.containers(all=True):
            if c['Names'] and 'figtest' in c['Names'][0]:
                self.client.kill(c['Id'])
                self.client.remove_container(c['Id'])
        for i in self.client.images():
            if isinstance(i.get('Tag'), basestring) and 'figtest' in i['Tag']:
                self.client.remove_image(i)

    def create_service(self, name, **kwargs):
        if 'command' not in kwargs:
            kwargs['command'] = ["/bin/sleep", "300"]
        return Service(
            project='figtest',
            name=name,
            client=self.client,
            image="busybox:latest",
            **kwargs
        )



