from __future__ import unicode_literals
from __future__ import absolute_import
from docker import Client
from fig.service import Service
from fig.cli.utils import docker_url
from . import unittest


class DockerClientTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.client = Client(docker_url())
        cls.client.pull('ubuntu')

    def setUp(self):
        for c in self.client.containers(all=True):
            if c['Names'] and 'figtest' in c['Names'][0]:
                self.client.kill(c['Id'])
                self.client.remove_container(c['Id'])

    def create_service(self, name, **kwargs):
        return Service(
            project='figtest',
            name=name,
            client=self.client,
            image="ubuntu",
            command=["/bin/sleep", "300"],
            **kwargs
        )



