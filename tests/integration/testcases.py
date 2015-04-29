from __future__ import unicode_literals
from __future__ import absolute_import
from compose.service import Service
from compose.config import make_service_dict
from compose.cli.docker_client import docker_client
from compose.progress_stream import stream_output
from .. import unittest


class DockerClientTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.client = docker_client()

    def setUp(self):
        for c in self.client.containers(all=True):
            if c['Names'] and 'composetest' in c['Names'][0]:
                self.client.kill(c['Id'])
                self.client.remove_container(c['Id'])
        for i in self.client.images():
            if isinstance(i.get('Tag'), basestring) and 'composetest' in i['Tag']:
                self.client.remove_image(i)

    def create_service(self, name, **kwargs):
        kwargs['image'] = kwargs.pop('image', 'busybox:latest')

        if 'command' not in kwargs:
            kwargs['command'] = ["/bin/sleep", "300"]

        return Service(
            project='composetest',
            client=self.client,
            **make_service_dict(name, kwargs, working_dir='.')
        )

    def check_build(self, *args, **kwargs):
        build_output = self.client.build(*args, **kwargs)
        stream_output(build_output, open('/dev/null', 'w'))
