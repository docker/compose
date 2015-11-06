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
            self.client.remove_container(c['Id'], force=True)
        for i in self.client.images(
                filters={'label': 'com.docker.compose.test_image'}):
            self.client.remove_image(i)

        for v in self.client.volumes()['Volumes']:
            if 'composetests_' in v['Name']:
                self.client.remove_volume(v['Name'])

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
        build_output = self.client.build(*args, **kwargs)
        stream_output(build_output, open('/dev/null', 'w'))

    def require_api_version(self, minimum):
        api_version = self.client.version()['ApiVersion']
        if version_lt(api_version, minimum):
            skip("API version is too low ({} < {})".format(api_version, minimum))
