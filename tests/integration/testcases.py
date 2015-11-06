from __future__ import absolute_import
from __future__ import unicode_literals

from docker import errors
from docker.utils import version_lt
from pytest import skip

from .. import unittest
from compose.cli.docker_client import docker_client
from compose.config.config import ServiceLoader
from compose.const import LABEL_PROJECT
from compose.progress_stream import stream_output
from compose.service import Service


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

        workaround_options = {}
        for option in ['links', 'volumes_from', 'net']:
            if option in kwargs:
                workaround_options[option] = kwargs.pop(option, None)

        options = ServiceLoader(
            working_dir='.',
            filename=None,
            service_name=name,
            service_dict=kwargs
        ).make_service_dict()
        options.update(workaround_options)

        labels = options.setdefault('labels', {})
        labels['com.docker.compose.test-name'] = self.id()

        return Service(project='composetest', client=self.client, **options)

    def check_build(self, *args, **kwargs):
        kwargs.setdefault('rm', True)
        build_output = self.client.build(*args, **kwargs)
        stream_output(build_output, open('/dev/null', 'w'))

    def require_api_version(self, minimum):
        api_version = self.client.version()['ApiVersion']
        if version_lt(api_version, minimum):
            skip("API version is too low ({} < {})".format(api_version, minimum))
