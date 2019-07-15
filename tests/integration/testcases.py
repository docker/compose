from __future__ import absolute_import
from __future__ import unicode_literals

import functools
import os

import pytest
from docker.errors import APIError
from docker.utils import version_lt

from .. import unittest
from ..helpers import BUSYBOX_IMAGE_WITH_TAG
from compose.cli.docker_client import docker_client
from compose.config.config import resolve_environment
from compose.config.environment import Environment
from compose.const import API_VERSIONS
from compose.const import COMPOSEFILE_V1 as V1
from compose.const import COMPOSEFILE_V2_0 as V2_0
from compose.const import COMPOSEFILE_V2_0 as V2_1
from compose.const import COMPOSEFILE_V2_2 as V2_2
from compose.const import COMPOSEFILE_V2_3 as V2_3
from compose.const import COMPOSEFILE_V3_0 as V3_0
from compose.const import COMPOSEFILE_V3_2 as V3_2
from compose.const import COMPOSEFILE_V3_5 as V3_5
from compose.const import LABEL_PROJECT
from compose.progress_stream import stream_output
from compose.service import Service

SWARM_SKIP_CONTAINERS_ALL = os.environ.get('SWARM_SKIP_CONTAINERS_ALL', '0') != '0'
SWARM_SKIP_CPU_SHARES = os.environ.get('SWARM_SKIP_CPU_SHARES', '0') != '0'
SWARM_SKIP_RM_VOLUMES = os.environ.get('SWARM_SKIP_RM_VOLUMES', '0') != '0'
SWARM_ASSUME_MULTINODE = os.environ.get('SWARM_ASSUME_MULTINODE', '0') != '0'


def pull_busybox(client):
    client.pull(BUSYBOX_IMAGE_WITH_TAG, stream=False)


def get_links(container):
    links = container.get('HostConfig.Links') or []

    def format_link(link):
        _, alias = link.split(':')
        return alias.split('/')[-1]

    return [format_link(link) for link in links]


def engine_max_version():
    if 'DOCKER_VERSION' not in os.environ:
        return V3_5
    version = os.environ['DOCKER_VERSION'].partition('-')[0]
    if version_lt(version, '1.10'):
        return V1
    if version_lt(version, '1.12'):
        return V2_0
    if version_lt(version, '1.13'):
        return V2_1
    if version_lt(version, '17.06'):
        return V3_2
    return V3_5


def min_version_skip(version):
    return pytest.mark.skipif(
        engine_max_version() < version,
        reason="Engine version %s is too low" % version
    )


def v2_only():
    return min_version_skip(V2_0)


def v2_1_only():
    return min_version_skip(V2_1)


def v2_2_only():
    return min_version_skip(V2_2)


def v2_3_only():
    return min_version_skip(V2_3)


def v3_only():
    return min_version_skip(V3_0)


class DockerClientTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        version = API_VERSIONS[engine_max_version()]
        cls.client = docker_client(Environment(), version)

    @classmethod
    def tearDownClass(cls):
        del cls.client

    def tearDown(self):
        for c in self.client.containers(
                all=True,
                filters={'label': '%s=composetest' % LABEL_PROJECT}):
            self.client.remove_container(c['Id'], force=True)

        for i in self.client.images(
                filters={'label': 'com.docker.compose.test_image'}):
            try:
                self.client.remove_image(i, force=True)
            except APIError as e:
                if e.is_server_error():
                    pass

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
            kwargs['image'] = BUSYBOX_IMAGE_WITH_TAG

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
        with open(os.devnull, 'w') as devnull:
            for event in stream_output(build_output, devnull):
                pass

    def require_api_version(self, minimum):
        api_version = self.client.version()['ApiVersion']
        if version_lt(api_version, minimum):
            pytest.skip("API version is too low ({} < {})".format(api_version, minimum))

    def get_volume_data(self, volume_name):
        if not is_cluster(self.client):
            return self.client.inspect_volume(volume_name)

        volumes = self.client.volumes(filters={'name': volume_name})['Volumes']
        assert len(volumes) > 0
        return self.client.inspect_volume(volumes[0]['Name'])


def if_runtime_available(runtime):
    def decorator(f):
        @functools.wraps(f)
        def wrapper(self, *args, **kwargs):
            if runtime not in self.client.info().get('Runtimes', {}):
                return pytest.skip("This daemon does not support the '{}'' runtime".format(runtime))
            return f(self, *args, **kwargs)
        return wrapper

    return decorator


def is_cluster(client):
    if SWARM_ASSUME_MULTINODE:
        return True

    def get_nodes_number():
        try:
            return len(client.nodes())
        except APIError:
            # If the Engine is not part of a Swarm, the SDK will raise
            # an APIError
            return 0

    if not hasattr(is_cluster, 'nodes') or is_cluster.nodes is None:
        # Only make the API call if the value hasn't been cached yet
        is_cluster.nodes = get_nodes_number()

    return is_cluster.nodes > 1


def no_cluster(reason):
    def decorator(f):
        @functools.wraps(f)
        def wrapper(self, *args, **kwargs):
            if is_cluster(self.client):
                pytest.skip("Test will not be run in cluster mode: %s" % reason)
                return
            return f(self, *args, **kwargs)
        return wrapper

    return decorator
