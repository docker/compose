from __future__ import absolute_import
from __future__ import unicode_literals

from .. import unittest
from compose.cli.docker_client import docker_client
from compose.config.config import ServiceLoader
from compose.const import LABEL_PROJECT
from compose.progress_stream import stream_output
from compose.service import Service


class DockerClientTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.client = docker_client()

    def tearDown(self):
        for c in self.client.containers(
                all=True,
                filters={'label': '%s=composetest' % LABEL_PROJECT}):
            self.client.kill(c['Id'])
            self.client.remove_container(c['Id'])
        for i in self.client.images(
                filters={'label': 'com.docker.compose.test_image'}):
            self.client.remove_image(i)

    def create_service(self, name, **kwargs):
        if 'image' not in kwargs and 'build' not in kwargs:
            kwargs['image'] = 'busybox:latest'

        if 'command' not in kwargs:
            kwargs['command'] = ["top"]

        options = ServiceLoader(working_dir='.').make_service_dict(name, kwargs)

        return Service(
            project='composetest',
            client=self.client,
            **options
        )

    def check_build(self, *args, **kwargs):
        kwargs.setdefault('rm', True)
        build_output = self.client.build(*args, **kwargs)
        stream_output(build_output, open('/dev/null', 'w'))
