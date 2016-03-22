from __future__ import absolute_import
from __future__ import unicode_literals

import functools
import os

from docker.utils import version_lt
from pytest import skip

from .. import unittest
from compose.cli.docker_client import docker_client
from compose.config.config import resolve_environment
from compose.config.config import V1
from compose.config.config import V2_0
from compose.config.environment import Environment
from compose.const import API_VERSIONS
from compose.const import LABEL_PROJECT
from compose.progress_stream import stream_output
from compose.service import Service


def pull_busybox(client):
    client.pull('busybox:latest', stream=False)


def get_links(container):
    links = container.get('HostConfig.Links') or []

    def format_link(link):
        _, alias = link.split(':')
        return alias.split('/')[-1]

    return [format_link(link) for link in links]


def engine_version_too_low_for_v2():
    if 'DOCKER_VERSION' not in os.environ:
        return False
    version = os.environ['DOCKER_VERSION'].partition('-')[0]
    return version_lt(version, '1.10')


def v2_only():
    def decorator(f):
        @functools.wraps(f)
        def wrapper(self, *args, **kwargs):
            if engine_version_too_low_for_v2():
                skip("Engine version is too low")
                return
            return f(self, *args, **kwargs)
        return wrapper

    return decorator


class DockerClientTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if engine_version_too_low_for_v2():
            version = API_VERSIONS[V1]
        else:
            version = API_VERSIONS[V2_0]

        cls.client = docker_client(Environment(), version)

    def tearDown(self):
        for c in self.client.containers(
                all=True,
                filters={'label': '%s=composetest' % LABEL_PROJECT}):
            self.client.remove_container(c['Id'], force=True)

        for i in self.client.images(
                filters={'label': 'com.docker.compose.test_image'}):
            self.client.remove_image(i)

        volumes = self.client.volumes().get('Volumes') or []
        for v in volumes:
            if 'composetest_' in v['Name']:
                self.client.remove_volume(v['Name'])

        networks = self.client.networks()
        for n in networks:
            if 'composetest_' in n['Name']:
                self.client.remove_network(n['Name'])

    def create_service(self, name, **kwargs):
        if 'image' not in kwargs and 'build' not in kwargs:
            kwargs['image'] = 'busybox:latest'

        if 'command' not in kwargs:
            kwargs['command'] = ["top"]

        kwargs['environment'] = resolve_environment(
            kwargs, Environment.from_env_file(None)
        )
        labels = dict(kwargs.setdefault('labels', {}))
        labels['com.docker.compose.test-name'] = self.id()

        return Service(name, client=self.client, project='composetest', **kwargs)

    def check_build(self, *args, **kwargs):
        kwargs.setdefault('rm', True)
        build_output = self.client.build(*args, **kwargs)
        stream_output(build_output, open('/dev/null', 'w'))

    def require_api_version(self, minimum):
        api_version = self.client.version()['ApiVersion']
        if version_lt(api_version, minimum):
            skip("API version is too low ({} < {})".format(api_version, minimum))
