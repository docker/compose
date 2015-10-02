from __future__ import absolute_import
from __future__ import unicode_literals

from docker import errors

from .. import unittest
from compose.cli.docker_client import docker_client
from compose.config.config import ServiceLoader
from compose.const import LABEL_PROJECT
from compose.progress_stream import stream_output
from compose.service import Service
from compose.utils import split_buffer
from compose.utils import stream_as_text


def pull_busybox(client):
    try:
        client.inspect_image('busybox:latest')
    except errors.APIError:
        client.pull('busybox:latest', stream=False)


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

        links = kwargs.get('links', None)
        volumes_from = kwargs.get('volumes_from', None)
        net = kwargs.get('net', None)

        workaround_options = ['links', 'volumes_from', 'net']
        for key in workaround_options:
            try:
                del kwargs[key]
            except KeyError:
                pass

        options = ServiceLoader(working_dir='.', filename=None, service_name=name, service_dict=kwargs).make_service_dict()

        labels = options.setdefault('labels', {})
        labels['com.docker.compose.test-name'] = self.id()

        if links:
            options['links'] = links
        if volumes_from:
            options['volumes_from'] = volumes_from
        if net:
            options['net'] = net

        return Service(
            project='composetest',
            client=self.client,
            **options
        )

    def check_build(self, *args, **kwargs):
        kwargs.setdefault('rm', True)
        build_output = stream_as_text(self.client.build(*args, **kwargs))
        stream_output(split_buffer(build_output), open('/dev/null', 'w'))
