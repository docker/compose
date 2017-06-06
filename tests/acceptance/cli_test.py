# -*- coding: utf-8 -*-
from __future__ import absolute_import
from __future__ import unicode_literals

import datetime
import json
import os
import os.path
import signal
import subprocess
import time
from collections import Counter
from collections import namedtuple
from operator import attrgetter

import py
import six
import yaml
from docker import errors

from .. import mock
from ..helpers import create_host_file
from compose.cli.command import get_project
from compose.config.errors import DuplicateOverrideFileFound
from compose.container import Container
from compose.project import OneOffFilter
from compose.utils import nanoseconds_from_time_seconds
from tests.integration.testcases import DockerClientTestCase
from tests.integration.testcases import get_links
from tests.integration.testcases import pull_busybox
from tests.integration.testcases import v2_1_only
from tests.integration.testcases import v2_only
from tests.integration.testcases import v3_only

ProcessResult = namedtuple('ProcessResult', 'stdout stderr')


BUILD_CACHE_TEXT = 'Using cache'
BUILD_PULL_TEXT = 'Status: Image is up to date for busybox:latest'


def start_process(base_dir, options):
    proc = subprocess.Popen(
        ['docker-compose'] + options,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        cwd=base_dir)
    print("Running process: %s" % proc.pid)
    return proc


def wait_on_process(proc, returncode=0):
    stdout, stderr = proc.communicate()
    if proc.returncode != returncode:
        print("Stderr: {}".format(stderr))
        print("Stdout: {}".format(stdout))
        assert proc.returncode == returncode
    return ProcessResult(stdout.decode('utf-8'), stderr.decode('utf-8'))


def wait_on_condition(condition, delay=0.1, timeout=40):
    start_time = time.time()
    while not condition():
        if time.time() - start_time > timeout:
            raise AssertionError("Timeout: %s" % condition)
        time.sleep(delay)


def kill_service(service):
    for container in service.containers():
        container.kill()


class ContainerCountCondition(object):

    def __init__(self, project, expected):
        self.project = project
        self.expected = expected

    def __call__(self):
        return len(self.project.containers()) == self.expected

    def __str__(self):
        return "waiting for counter count == %s" % self.expected


class ContainerStateCondition(object):

    def __init__(self, client, name, status):
        self.client = client
        self.name = name
        self.status = status

    def __call__(self):
        try:
            container = self.client.inspect_container(self.name)
            return container['State']['Status'] == self.status
        except errors.APIError:
            return False

    def __str__(self):
        return "waiting for container to be %s" % self.status


class CLITestCase(DockerClientTestCase):

    def setUp(self):
        super(CLITestCase, self).setUp()
        self.base_dir = 'tests/fixtures/simple-composefile'
        self.override_dir = None

    def tearDown(self):
        if self.base_dir:
            self.project.kill()
            self.project.remove_stopped()

            for container in self.project.containers(stopped=True, one_off=OneOffFilter.only):
                container.remove(force=True)

            networks = self.client.networks()
            for n in networks:
                if n['Name'].startswith('{}_'.format(self.project.name)):
                    self.client.remove_network(n['Name'])
        if hasattr(self, '_project'):
            del self._project

        super(CLITestCase, self).tearDown()

    @property
    def project(self):
        # Hack: allow project to be overridden
        if not hasattr(self, '_project'):
            self._project = get_project(self.base_dir, override_dir=self.override_dir)
        return self._project

    def dispatch(self, options, project_options=None, returncode=0):
        project_options = project_options or []
        proc = start_process(self.base_dir, project_options + options)
        return wait_on_process(proc, returncode=returncode)

    def execute(self, container, cmd):
        # Remove once Hijack and CloseNotifier sign a peace treaty
        self.client.close()
        exc = self.client.exec_create(container.id, cmd)
        self.client.exec_start(exc)
        return self.client.exec_inspect(exc)['ExitCode']

    def lookup(self, container, hostname):
        return self.execute(container, ["nslookup", hostname]) == 0

    def test_help(self):
        self.base_dir = 'tests/fixtures/no-composefile'
        result = self.dispatch(['help', 'up'], returncode=0)
        assert 'Usage: up [options] [--scale SERVICE=NUM...] [SERVICE...]' in result.stdout
        # Prevent tearDown from trying to create a project
        self.base_dir = None

    def test_help_nonexistent(self):
        self.base_dir = 'tests/fixtures/no-composefile'
        result = self.dispatch(['help', 'foobar'], returncode=1)
        assert 'No such command' in result.stderr
        self.base_dir = None

    def test_shorthand_host_opt(self):
        self.dispatch(
            ['-H={0}'.format(os.environ.get('DOCKER_HOST', 'unix://')),
             'up', '-d'],
            returncode=0
        )

    def test_host_not_reachable(self):
        result = self.dispatch(['-H=tcp://doesnotexist:8000', 'ps'], returncode=1)
        assert "Couldn't connect to Docker daemon" in result.stderr

    def test_host_not_reachable_volumes_from_container(self):
        self.base_dir = 'tests/fixtures/volumes-from-container'

        container = self.client.create_container('busybox', 'true', name='composetest_data_container')
        self.addCleanup(self.client.remove_container, container)

        result = self.dispatch(['-H=tcp://doesnotexist:8000', 'ps'], returncode=1)
        assert "Couldn't connect to Docker daemon" in result.stderr

    def test_config_list_services(self):
        self.base_dir = 'tests/fixtures/v2-full'
        result = self.dispatch(['config', '--services'])
        assert set(result.stdout.rstrip().split('\n')) == {'web', 'other'}

    def test_config_list_volumes(self):
        self.base_dir = 'tests/fixtures/v2-full'
        result = self.dispatch(['config', '--volumes'])
        assert set(result.stdout.rstrip().split('\n')) == {'data'}

    def test_config_quiet_with_error(self):
        self.base_dir = None
        result = self.dispatch([
            '-f', 'tests/fixtures/invalid-composefile/invalid.yml',
            'config', '-q'
        ], returncode=1)
        assert "'notaservice' must be a mapping" in result.stderr

    def test_config_quiet(self):
        self.base_dir = 'tests/fixtures/v2-full'
        assert self.dispatch(['config', '-q']).stdout == ''

    def test_config_default(self):
        self.base_dir = 'tests/fixtures/v2-full'
        result = self.dispatch(['config'])
        # assert there are no python objects encoded in the output
        assert '!!' not in result.stdout

        output = yaml.load(result.stdout)
        expected = {
            'version': '2.0',
            'volumes': {'data': {'driver': 'local'}},
            'networks': {'front': {}},
            'services': {
                'web': {
                    'build': {
                        'context': os.path.abspath(self.base_dir),
                    },
                    'networks': {'front': None, 'default': None},
                    'volumes_from': ['service:other:rw'],
                },
                'other': {
                    'image': 'busybox:latest',
                    'command': 'top',
                    'volumes': ['/data'],
                },
            },
        }
        assert output == expected

    def test_config_restart(self):
        self.base_dir = 'tests/fixtures/restart'
        result = self.dispatch(['config'])
        assert yaml.load(result.stdout) == {
            'version': '2.0',
            'services': {
                'never': {
                    'image': 'busybox',
                    'restart': 'no',
                },
                'always': {
                    'image': 'busybox',
                    'restart': 'always',
                },
                'on-failure': {
                    'image': 'busybox',
                    'restart': 'on-failure',
                },
                'on-failure-5': {
                    'image': 'busybox',
                    'restart': 'on-failure:5',
                },
                'restart-null': {
                    'image': 'busybox',
                    'restart': ''
                },
            },
        }

    def test_config_external_network(self):
        self.base_dir = 'tests/fixtures/networks'
        result = self.dispatch(['-f', 'external-networks.yml', 'config'])
        json_result = yaml.load(result.stdout)
        assert 'networks' in json_result
        assert json_result['networks'] == {
            'networks_foo': {
                'external': True  # {'name': 'networks_foo'}
            },
            'bar': {
                'external': {'name': 'networks_bar'}
            }
        }

    def test_config_external_volume(self):
        self.base_dir = 'tests/fixtures/volumes'
        result = self.dispatch(['-f', 'external-volumes.yml', 'config'])
        json_result = yaml.load(result.stdout)
        assert 'volumes' in json_result
        assert json_result['volumes'] == {
            'foo': {
                'external': True
            },
            'bar': {
                'external': {'name': 'some_bar'}
            }
        }

    def test_config_v1(self):
        self.base_dir = 'tests/fixtures/v1-config'
        result = self.dispatch(['config'])
        assert yaml.load(result.stdout) == {
            'version': '2.1',
            'services': {
                'net': {
                    'image': 'busybox',
                    'network_mode': 'bridge',
                },
                'volume': {
                    'image': 'busybox',
                    'volumes': ['/data'],
                    'network_mode': 'bridge',
                },
                'app': {
                    'image': 'busybox',
                    'volumes_from': ['service:volume:rw'],
                    'network_mode': 'service:net',
                },
            },
        }

    @v3_only()
    def test_config_v3(self):
        self.base_dir = 'tests/fixtures/v3-full'
        result = self.dispatch(['config'])

        assert yaml.load(result.stdout) == {
            'version': '3.2',
            'volumes': {
                'foobar': {
                    'labels': {
                        'com.docker.compose.test': 'true',
                    },
                },
            },
            'services': {
                'web': {
                    'image': 'busybox',
                    'deploy': {
                        'mode': 'replicated',
                        'replicas': 6,
                        'labels': ['FOO=BAR'],
                        'update_config': {
                            'parallelism': 3,
                            'delay': '10s',
                            'failure_action': 'continue',
                            'monitor': '60s',
                            'max_failure_ratio': 0.3,
                        },
                        'resources': {
                            'limits': {
                                'cpus': '0.001',
                                'memory': '50M',
                            },
                            'reservations': {
                                'cpus': '0.0001',
                                'memory': '20M',
                            },
                        },
                        'restart_policy': {
                            'condition': 'on_failure',
                            'delay': '5s',
                            'max_attempts': 3,
                            'window': '120s',
                        },
                        'placement': {
                            'constraints': ['node=foo'],
                        },
                    },

                    'healthcheck': {
                        'test': 'cat /etc/passwd',
                        'interval': '10s',
                        'timeout': '1s',
                        'retries': 5,
                    },
                    'volumes': [
                        '/host/path:/container/path:ro',
                        'foobar:/container/volumepath:rw',
                        '/anonymous',
                        'foobar:/container/volumepath2:nocopy'
                    ],

                    'stop_grace_period': '20s',
                },
            },
        }

    def test_ps(self):
        self.project.get_service('simple').create_container()
        result = self.dispatch(['ps'])
        assert 'simplecomposefile_simple_1' in result.stdout

    def test_ps_default_composefile(self):
        self.base_dir = 'tests/fixtures/multiple-composefiles'
        self.dispatch(['up', '-d'])
        result = self.dispatch(['ps'])

        self.assertIn('multiplecomposefiles_simple_1', result.stdout)
        self.assertIn('multiplecomposefiles_another_1', result.stdout)
        self.assertNotIn('multiplecomposefiles_yetanother_1', result.stdout)

    def test_ps_alternate_composefile(self):
        config_path = os.path.abspath(
            'tests/fixtures/multiple-composefiles/compose2.yml')
        self._project = get_project(self.base_dir, [config_path])

        self.base_dir = 'tests/fixtures/multiple-composefiles'
        self.dispatch(['-f', 'compose2.yml', 'up', '-d'])
        result = self.dispatch(['-f', 'compose2.yml', 'ps'])

        self.assertNotIn('multiplecomposefiles_simple_1', result.stdout)
        self.assertNotIn('multiplecomposefiles_another_1', result.stdout)
        self.assertIn('multiplecomposefiles_yetanother_1', result.stdout)

    def test_pull(self):
        result = self.dispatch(['pull'])
        assert sorted(result.stderr.split('\n'))[1:] == [
            'Pulling another (busybox:latest)...',
            'Pulling simple (busybox:latest)...',
        ]

    def test_pull_with_digest(self):
        result = self.dispatch(['-f', 'digest.yml', 'pull'])

        assert 'Pulling simple (busybox:latest)...' in result.stderr
        assert ('Pulling digest (busybox@'
                'sha256:38a203e1986cf79639cfb9b2e1d6e773de84002feea2d4eb006b520'
                '04ee8502d)...') in result.stderr

    def test_pull_with_ignore_pull_failures(self):
        result = self.dispatch([
            '-f', 'ignore-pull-failures.yml',
            'pull', '--ignore-pull-failures']
        )

        assert 'Pulling simple (busybox:latest)...' in result.stderr
        assert 'Pulling another (nonexisting-image:latest)...' in result.stderr
        assert ('repository nonexisting-image not found' in result.stderr or
                'image library/nonexisting-image:latest not found' in result.stderr)

    def test_build_plain(self):
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', 'simple'])

        result = self.dispatch(['build', 'simple'])
        assert BUILD_CACHE_TEXT in result.stdout
        assert BUILD_PULL_TEXT not in result.stdout

    def test_build_no_cache(self):
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', 'simple'])

        result = self.dispatch(['build', '--no-cache', 'simple'])
        assert BUILD_CACHE_TEXT not in result.stdout
        assert BUILD_PULL_TEXT not in result.stdout

    def test_build_pull(self):
        # Make sure we have the latest busybox already
        pull_busybox(self.client)
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', 'simple'], None)

        result = self.dispatch(['build', '--pull', 'simple'])
        assert BUILD_CACHE_TEXT in result.stdout
        assert BUILD_PULL_TEXT in result.stdout

    def test_build_no_cache_pull(self):
        # Make sure we have the latest busybox already
        pull_busybox(self.client)
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', 'simple'])

        result = self.dispatch(['build', '--no-cache', '--pull', 'simple'])
        assert BUILD_CACHE_TEXT not in result.stdout
        assert BUILD_PULL_TEXT in result.stdout

    def test_build_failed(self):
        self.base_dir = 'tests/fixtures/simple-failing-dockerfile'
        self.dispatch(['build', 'simple'], returncode=1)

        labels = ["com.docker.compose.test_failing_image=true"]
        containers = [
            Container.from_ps(self.project.client, c)
            for c in self.project.client.containers(
                all=True,
                filters={"label": labels})
        ]
        assert len(containers) == 1

    def test_build_failed_forcerm(self):
        self.base_dir = 'tests/fixtures/simple-failing-dockerfile'
        self.dispatch(['build', '--force-rm', 'simple'], returncode=1)

        labels = ["com.docker.compose.test_failing_image=true"]

        containers = [
            Container.from_ps(self.project.client, c)
            for c in self.project.client.containers(
                all=True,
                filters={"label": labels})
        ]
        assert not containers

    def test_bundle_with_digests(self):
        self.base_dir = 'tests/fixtures/bundle-with-digests/'
        tmpdir = py.test.ensuretemp('cli_test_bundle')
        self.addCleanup(tmpdir.remove)
        filename = str(tmpdir.join('example.dab'))

        self.dispatch(['bundle', '--output', filename])
        with open(filename, 'r') as fh:
            bundle = json.load(fh)

        assert bundle == {
            'Version': '0.1',
            'Services': {
                'web': {
                    'Image': ('dockercloud/hello-world@sha256:fe79a2cfbd17eefc3'
                              '44fb8419420808df95a1e22d93b7f621a7399fd1e9dca1d'),
                    'Networks': ['default'],
                },
                'redis': {
                    'Image': ('redis@sha256:a84cb8f53a70e19f61ff2e1d5e73fb7ae62d'
                              '374b2b7392de1e7d77be26ef8f7b'),
                    'Networks': ['default'],
                }
            },
        }

    def test_build_override_dir(self):
        self.base_dir = 'tests/fixtures/build-path-override-dir'
        self.override_dir = os.path.abspath('tests/fixtures')
        result = self.dispatch([
            '--project-directory', self.override_dir,
            'build'])

        assert 'Successfully built' in result.stdout

    def test_build_override_dir_invalid_path(self):
        config_path = os.path.abspath('tests/fixtures/build-path-override-dir/docker-compose.yml')
        result = self.dispatch([
            '-f', config_path,
            'build'], returncode=1)

        assert 'does not exist, is not accessible, or is not a valid URL' in result.stderr

    def test_create(self):
        self.dispatch(['create'])
        service = self.project.get_service('simple')
        another = self.project.get_service('another')
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(another.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertEqual(len(another.containers(stopped=True)), 1)

    def test_create_with_force_recreate(self):
        self.dispatch(['create'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)

        old_ids = [c.id for c in service.containers(stopped=True)]

        self.dispatch(['create', '--force-recreate'], None)
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)

        new_ids = [c.id for c in service.containers(stopped=True)]

        self.assertNotEqual(old_ids, new_ids)

    def test_create_with_no_recreate(self):
        self.dispatch(['create'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)

        old_ids = [c.id for c in service.containers(stopped=True)]

        self.dispatch(['create', '--no-recreate'], None)
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)

        new_ids = [c.id for c in service.containers(stopped=True)]

        self.assertEqual(old_ids, new_ids)

    def test_run_one_off_with_volume(self):
        self.base_dir = 'tests/fixtures/simple-composefile-volume-ready'
        volume_path = os.path.abspath(os.path.join(os.getcwd(), self.base_dir, 'files'))
        create_host_file(self.client, os.path.join(volume_path, 'example.txt'))

        self.dispatch([
            'run',
            '-v', '{}:/data'.format(volume_path),
            'simple',
            'test', '-f', '/data/example.txt'
        ], returncode=0)
        # FIXME: does not work with Python 3
        # assert cmd_result.stdout.strip() == 'FILE_CONTENT'

    def test_run_one_off_with_multiple_volumes(self):
        self.base_dir = 'tests/fixtures/simple-composefile-volume-ready'
        volume_path = os.path.abspath(os.path.join(os.getcwd(), self.base_dir, 'files'))
        create_host_file(self.client, os.path.join(volume_path, 'example.txt'))

        self.dispatch([
            'run',
            '-v', '{}:/data'.format(volume_path),
            '-v', '{}:/data1'.format(volume_path),
            'simple',
            'test', '-f', '/data/example.txt'
        ], returncode=0)
        # FIXME: does not work with Python 3
        # assert cmd_result.stdout.strip() == 'FILE_CONTENT'

        self.dispatch([
            'run',
            '-v', '{}:/data'.format(volume_path),
            '-v', '{}:/data1'.format(volume_path),
            'simple',
            'test', '-f' '/data1/example.txt'
        ], returncode=0)
        # FIXME: does not work with Python 3
        # assert cmd_result.stdout.strip() == 'FILE_CONTENT'

    def test_create_with_force_recreate_and_no_recreate(self):
        self.dispatch(
            ['create', '--force-recreate', '--no-recreate'],
            returncode=1)

    def test_down_invalid_rmi_flag(self):
        result = self.dispatch(['down', '--rmi', 'bogus'], returncode=1)
        assert '--rmi flag must be' in result.stderr

    @v2_only()
    def test_down(self):
        self.base_dir = 'tests/fixtures/v2-full'

        self.dispatch(['up', '-d'])
        wait_on_condition(ContainerCountCondition(self.project, 2))

        self.dispatch(['run', 'web', 'true'])
        self.dispatch(['run', '-d', 'web', 'tail', '-f', '/dev/null'])
        assert len(self.project.containers(one_off=OneOffFilter.only, stopped=True)) == 2

        result = self.dispatch(['down', '--rmi=local', '--volumes'])
        assert 'Stopping v2full_web_1' in result.stderr
        assert 'Stopping v2full_other_1' in result.stderr
        assert 'Stopping v2full_web_run_2' in result.stderr
        assert 'Removing v2full_web_1' in result.stderr
        assert 'Removing v2full_other_1' in result.stderr
        assert 'Removing v2full_web_run_1' in result.stderr
        assert 'Removing v2full_web_run_2' in result.stderr
        assert 'Removing volume v2full_data' in result.stderr
        assert 'Removing image v2full_web' in result.stderr
        assert 'Removing image busybox' not in result.stderr
        assert 'Removing network v2full_default' in result.stderr
        assert 'Removing network v2full_front' in result.stderr

    def test_up_detached(self):
        self.dispatch(['up', '-d'])
        service = self.project.get_service('simple')
        another = self.project.get_service('another')
        self.assertEqual(len(service.containers()), 1)
        self.assertEqual(len(another.containers()), 1)

        # Ensure containers don't have stdin and stdout connected in -d mode
        container, = service.containers()
        self.assertFalse(container.get('Config.AttachStderr'))
        self.assertFalse(container.get('Config.AttachStdout'))
        self.assertFalse(container.get('Config.AttachStdin'))

    def test_up_attached(self):
        self.base_dir = 'tests/fixtures/echo-services'
        result = self.dispatch(['up', '--no-color'])

        assert 'simple_1   | simple' in result.stdout
        assert 'another_1  | another' in result.stdout
        assert 'simple_1 exited with code 0' in result.stdout
        assert 'another_1 exited with code 0' in result.stdout

    @v2_only()
    def test_up(self):
        self.base_dir = 'tests/fixtures/v2-simple'
        self.dispatch(['up', '-d'], None)

        services = self.project.get_services()

        network_name = self.project.networks.networks['default'].full_name
        networks = self.client.networks(names=[network_name])
        self.assertEqual(len(networks), 1)
        self.assertEqual(networks[0]['Driver'], 'bridge')
        assert 'com.docker.network.bridge.enable_icc' not in networks[0]['Options']

        network = self.client.inspect_network(networks[0]['Id'])

        for service in services:
            containers = service.containers()
            self.assertEqual(len(containers), 1)

            container = containers[0]
            self.assertIn(container.id, network['Containers'])

            networks = container.get('NetworkSettings.Networks')
            self.assertEqual(list(networks), [network['Name']])

            self.assertEqual(
                sorted(networks[network['Name']]['Aliases']),
                sorted([service.name, container.short_id]))

            for service in services:
                assert self.lookup(container, service.name)

    @v2_only()
    def test_up_with_default_network_config(self):
        filename = 'default-network-config.yml'

        self.base_dir = 'tests/fixtures/networks'
        self._project = get_project(self.base_dir, [filename])

        self.dispatch(['-f', filename, 'up', '-d'], None)

        network_name = self.project.networks.networks['default'].full_name
        networks = self.client.networks(names=[network_name])

        assert networks[0]['Options']['com.docker.network.bridge.enable_icc'] == 'false'

    @v2_only()
    def test_up_with_network_aliases(self):
        filename = 'network-aliases.yml'
        self.base_dir = 'tests/fixtures/networks'
        self.dispatch(['-f', filename, 'up', '-d'], None)
        back_name = '{}_back'.format(self.project.name)
        front_name = '{}_front'.format(self.project.name)

        networks = [
            n for n in self.client.networks()
            if n['Name'].startswith('{}_'.format(self.project.name))
        ]

        # Two networks were created: back and front
        assert sorted(n['Name'] for n in networks) == [back_name, front_name]
        web_container = self.project.get_service('web').containers()[0]

        back_aliases = web_container.get(
            'NetworkSettings.Networks.{}.Aliases'.format(back_name)
        )
        assert 'web' in back_aliases
        front_aliases = web_container.get(
            'NetworkSettings.Networks.{}.Aliases'.format(front_name)
        )
        assert 'web' in front_aliases
        assert 'forward_facing' in front_aliases
        assert 'ahead' in front_aliases

    @v2_only()
    def test_up_with_network_internal(self):
        self.require_api_version('1.23')
        filename = 'network-internal.yml'
        self.base_dir = 'tests/fixtures/networks'
        self.dispatch(['-f', filename, 'up', '-d'], None)
        internal_net = '{}_internal'.format(self.project.name)

        networks = [
            n for n in self.client.networks()
            if n['Name'].startswith('{}_'.format(self.project.name))
        ]

        # One network was created: internal
        assert sorted(n['Name'] for n in networks) == [internal_net]

        assert networks[0]['Internal'] is True

    @v2_only()
    def test_up_with_network_static_addresses(self):
        filename = 'network-static-addresses.yml'
        ipv4_address = '172.16.100.100'
        ipv6_address = 'fe80::1001:100'
        self.base_dir = 'tests/fixtures/networks'
        self.dispatch(['-f', filename, 'up', '-d'], None)
        static_net = '{}_static_test'.format(self.project.name)

        networks = [
            n for n in self.client.networks()
            if n['Name'].startswith('{}_'.format(self.project.name))
        ]

        # One networks was created: front
        assert sorted(n['Name'] for n in networks) == [static_net]
        web_container = self.project.get_service('web').containers()[0]

        ipam_config = web_container.get(
            'NetworkSettings.Networks.{}.IPAMConfig'.format(static_net)
        )
        assert ipv4_address in ipam_config.values()
        assert ipv6_address in ipam_config.values()

    @v2_only()
    def test_up_with_networks(self):
        self.base_dir = 'tests/fixtures/networks'
        self.dispatch(['up', '-d'], None)

        back_name = '{}_back'.format(self.project.name)
        front_name = '{}_front'.format(self.project.name)

        networks = [
            n for n in self.client.networks()
            if n['Name'].startswith('{}_'.format(self.project.name))
        ]

        # Two networks were created: back and front
        assert sorted(n['Name'] for n in networks) == [back_name, front_name]

        back_network = [n for n in networks if n['Name'] == back_name][0]
        front_network = [n for n in networks if n['Name'] == front_name][0]

        web_container = self.project.get_service('web').containers()[0]
        app_container = self.project.get_service('app').containers()[0]
        db_container = self.project.get_service('db').containers()[0]

        for net_name in [front_name, back_name]:
            links = app_container.get('NetworkSettings.Networks.{}.Links'.format(net_name))
            assert '{}:database'.format(db_container.name) in links

        # db and app joined the back network
        assert sorted(back_network['Containers']) == sorted([db_container.id, app_container.id])

        # web and app joined the front network
        assert sorted(front_network['Containers']) == sorted([web_container.id, app_container.id])

        # web can see app but not db
        assert self.lookup(web_container, "app")
        assert not self.lookup(web_container, "db")

        # app can see db
        assert self.lookup(app_container, "db")

        # app has aliased db to "database"
        assert self.lookup(app_container, "database")

    @v2_only()
    def test_up_missing_network(self):
        self.base_dir = 'tests/fixtures/networks'

        result = self.dispatch(
            ['-f', 'missing-network.yml', 'up', '-d'],
            returncode=1)

        assert 'Service "web" uses an undefined network "foo"' in result.stderr

    @v2_only()
    def test_up_with_network_mode(self):
        c = self.client.create_container('busybox', 'top', name='composetest_network_mode_container')
        self.addCleanup(self.client.remove_container, c, force=True)
        self.client.start(c)
        container_mode_source = 'container:{}'.format(c['Id'])

        filename = 'network-mode.yml'

        self.base_dir = 'tests/fixtures/networks'
        self._project = get_project(self.base_dir, [filename])

        self.dispatch(['-f', filename, 'up', '-d'], None)

        networks = [
            n for n in self.client.networks()
            if n['Name'].startswith('{}_'.format(self.project.name))
        ]
        assert not networks

        for name in ['bridge', 'host', 'none']:
            container = self.project.get_service(name).containers()[0]
            assert list(container.get('NetworkSettings.Networks')) == [name]
            assert container.get('HostConfig.NetworkMode') == name

        service_mode_source = 'container:{}'.format(
            self.project.get_service('bridge').containers()[0].id)
        service_mode_container = self.project.get_service('service').containers()[0]
        assert not service_mode_container.get('NetworkSettings.Networks')
        assert service_mode_container.get('HostConfig.NetworkMode') == service_mode_source

        container_mode_container = self.project.get_service('container').containers()[0]
        assert not container_mode_container.get('NetworkSettings.Networks')
        assert container_mode_container.get('HostConfig.NetworkMode') == container_mode_source

    @v2_only()
    def test_up_external_networks(self):
        filename = 'external-networks.yml'

        self.base_dir = 'tests/fixtures/networks'
        self._project = get_project(self.base_dir, [filename])

        result = self.dispatch(['-f', filename, 'up', '-d'], returncode=1)
        assert 'declared as external, but could not be found' in result.stderr

        networks = [
            n['Name'] for n in self.client.networks()
            if n['Name'].startswith('{}_'.format(self.project.name))
        ]
        assert not networks

        network_names = ['{}_{}'.format(self.project.name, n) for n in ['foo', 'bar']]
        for name in network_names:
            self.client.create_network(name)

        self.dispatch(['-f', filename, 'up', '-d'])
        container = self.project.containers()[0]
        assert sorted(list(container.get('NetworkSettings.Networks'))) == sorted(network_names)

    @v2_only()
    def test_up_with_external_default_network(self):
        filename = 'external-default.yml'

        self.base_dir = 'tests/fixtures/networks'
        self._project = get_project(self.base_dir, [filename])

        result = self.dispatch(['-f', filename, 'up', '-d'], returncode=1)
        assert 'declared as external, but could not be found' in result.stderr

        networks = [
            n['Name'] for n in self.client.networks()
            if n['Name'].startswith('{}_'.format(self.project.name))
        ]
        assert not networks

        network_name = 'composetest_external_network'
        self.client.create_network(network_name)

        self.dispatch(['-f', filename, 'up', '-d'])
        container = self.project.containers()[0]
        assert list(container.get('NetworkSettings.Networks')) == [network_name]

    @v2_1_only()
    def test_up_with_network_labels(self):
        filename = 'network-label.yml'

        self.base_dir = 'tests/fixtures/networks'
        self._project = get_project(self.base_dir, [filename])

        self.dispatch(['-f', filename, 'up', '-d'], returncode=0)

        network_with_label = '{}_network_with_label'.format(self.project.name)

        networks = [
            n for n in self.client.networks()
            if n['Name'].startswith('{}_'.format(self.project.name))
        ]

        assert [n['Name'] for n in networks] == [network_with_label]
        assert 'label_key' in networks[0]['Labels']
        assert networks[0]['Labels']['label_key'] == 'label_val'

    @v2_1_only()
    def test_up_with_volume_labels(self):
        filename = 'volume-label.yml'

        self.base_dir = 'tests/fixtures/volumes'
        self._project = get_project(self.base_dir, [filename])

        self.dispatch(['-f', filename, 'up', '-d'], returncode=0)

        volume_with_label = '{}_volume_with_label'.format(self.project.name)

        volumes = [
            v for v in self.client.volumes().get('Volumes', [])
            if v['Name'].startswith('{}_'.format(self.project.name))
        ]

        assert [v['Name'] for v in volumes] == [volume_with_label]
        assert 'label_key' in volumes[0]['Labels']
        assert volumes[0]['Labels']['label_key'] == 'label_val'

    @v2_only()
    def test_up_no_services(self):
        self.base_dir = 'tests/fixtures/no-services'
        self.dispatch(['up', '-d'], None)

        network_names = [
            n['Name'] for n in self.client.networks()
            if n['Name'].startswith('{}_'.format(self.project.name))
        ]
        assert network_names == []

    def test_up_with_links_v1(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d', 'web'], None)

        # No network was created
        network_name = self.project.networks.networks['default'].full_name
        networks = self.client.networks(names=[network_name])
        assert networks == []

        web = self.project.get_service('web')
        db = self.project.get_service('db')
        console = self.project.get_service('console')

        # console was not started
        self.assertEqual(len(web.containers()), 1)
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(console.containers()), 0)

        # web has links
        web_container = web.containers()[0]
        self.assertTrue(web_container.get('HostConfig.Links'))

    def test_up_with_net_is_invalid(self):
        self.base_dir = 'tests/fixtures/net-container'

        result = self.dispatch(
            ['-f', 'v2-invalid.yml', 'up', '-d'],
            returncode=1)

        assert "Unsupported config option for services.bar: 'net'" in result.stderr

    def test_up_with_net_v1(self):
        self.base_dir = 'tests/fixtures/net-container'
        self.dispatch(['up', '-d'], None)

        bar = self.project.get_service('bar')
        bar_container = bar.containers()[0]

        foo = self.project.get_service('foo')
        foo_container = foo.containers()[0]

        assert foo_container.get('HostConfig.NetworkMode') == \
            'container:{}'.format(bar_container.id)

    @v3_only()
    def test_up_with_healthcheck(self):
        def wait_on_health_status(container, status):
            def condition():
                container.inspect()
                return container.get('State.Health.Status') == status

            return wait_on_condition(condition, delay=0.5)

        self.base_dir = 'tests/fixtures/healthcheck'
        self.dispatch(['up', '-d'], None)

        passes = self.project.get_service('passes')
        passes_container = passes.containers()[0]

        assert passes_container.get('Config.Healthcheck') == {
            "Test": ["CMD-SHELL", "/bin/true"],
            "Interval": nanoseconds_from_time_seconds(1),
            "Timeout": nanoseconds_from_time_seconds(30 * 60),
            "Retries": 1,
        }

        wait_on_health_status(passes_container, 'healthy')

        fails = self.project.get_service('fails')
        fails_container = fails.containers()[0]

        assert fails_container.get('Config.Healthcheck') == {
            "Test": ["CMD", "/bin/false"],
            "Interval": nanoseconds_from_time_seconds(2.5),
            "Retries": 2,
        }

        wait_on_health_status(fails_container, 'unhealthy')

        disabled = self.project.get_service('disabled')
        disabled_container = disabled.containers()[0]

        assert disabled_container.get('Config.Healthcheck') == {
            "Test": ["NONE"],
        }

        assert 'Health' not in disabled_container.get('State')

    def test_up_with_no_deps(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d', '--no-deps', 'web'], None)
        web = self.project.get_service('web')
        db = self.project.get_service('db')
        console = self.project.get_service('console')
        self.assertEqual(len(web.containers()), 1)
        self.assertEqual(len(db.containers()), 0)
        self.assertEqual(len(console.containers()), 0)

    def test_up_with_force_recreate(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)

        old_ids = [c.id for c in service.containers()]

        self.dispatch(['up', '-d', '--force-recreate'], None)
        self.assertEqual(len(service.containers()), 1)

        new_ids = [c.id for c in service.containers()]

        self.assertNotEqual(old_ids, new_ids)

    def test_up_with_no_recreate(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)

        old_ids = [c.id for c in service.containers()]

        self.dispatch(['up', '-d', '--no-recreate'], None)
        self.assertEqual(len(service.containers()), 1)

        new_ids = [c.id for c in service.containers()]

        self.assertEqual(old_ids, new_ids)

    def test_up_with_force_recreate_and_no_recreate(self):
        self.dispatch(
            ['up', '-d', '--force-recreate', '--no-recreate'],
            returncode=1)

    def test_up_with_timeout(self):
        self.dispatch(['up', '-d', '-t', '1'])
        service = self.project.get_service('simple')
        another = self.project.get_service('another')
        self.assertEqual(len(service.containers()), 1)
        self.assertEqual(len(another.containers()), 1)

        # Ensure containers don't have stdin and stdout connected in -d mode
        config = service.containers()[0].inspect()['Config']
        self.assertFalse(config['AttachStderr'])
        self.assertFalse(config['AttachStdout'])
        self.assertFalse(config['AttachStdin'])

    def test_up_handles_sigint(self):
        proc = start_process(self.base_dir, ['up', '-t', '2'])
        wait_on_condition(ContainerCountCondition(self.project, 2))

        os.kill(proc.pid, signal.SIGINT)
        wait_on_condition(ContainerCountCondition(self.project, 0))

    def test_up_handles_sigterm(self):
        proc = start_process(self.base_dir, ['up', '-t', '2'])
        wait_on_condition(ContainerCountCondition(self.project, 2))

        os.kill(proc.pid, signal.SIGTERM)
        wait_on_condition(ContainerCountCondition(self.project, 0))

    @v2_only()
    def test_up_handles_force_shutdown(self):
        self.base_dir = 'tests/fixtures/sleeps-composefile'
        proc = start_process(self.base_dir, ['up', '-t', '200'])
        wait_on_condition(ContainerCountCondition(self.project, 2))

        os.kill(proc.pid, signal.SIGTERM)
        time.sleep(0.1)
        os.kill(proc.pid, signal.SIGTERM)
        wait_on_condition(ContainerCountCondition(self.project, 0))

    def test_up_handles_abort_on_container_exit(self):
        self.base_dir = 'tests/fixtures/abort-on-container-exit-0'
        proc = start_process(self.base_dir, ['up', '--abort-on-container-exit'])
        wait_on_condition(ContainerCountCondition(self.project, 0))
        proc.wait()
        self.assertEqual(proc.returncode, 0)

    def test_up_handles_abort_on_container_exit_code(self):
        self.base_dir = 'tests/fixtures/abort-on-container-exit-1'
        proc = start_process(self.base_dir, ['up', '--abort-on-container-exit'])
        wait_on_condition(ContainerCountCondition(self.project, 0))
        proc.wait()
        self.assertEqual(proc.returncode, 1)

    def test_exec_without_tty(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d', 'console'])
        self.assertEqual(len(self.project.containers()), 1)

        stdout, stderr = self.dispatch(['exec', '-T', 'console', 'ls', '-1d', '/'])
        self.assertEqual(stdout, "/\n")
        self.assertEqual(stderr, "")

    def test_exec_custom_user(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d', 'console'])
        self.assertEqual(len(self.project.containers()), 1)

        stdout, stderr = self.dispatch(['exec', '-T', '--user=operator', 'console', 'whoami'])
        self.assertEqual(stdout, "operator\n")
        self.assertEqual(stderr, "")

    def test_run_service_without_links(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['run', 'console', '/bin/true'])
        self.assertEqual(len(self.project.containers()), 0)

        # Ensure stdin/out was open
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        config = container.inspect()['Config']
        self.assertTrue(config['AttachStderr'])
        self.assertTrue(config['AttachStdout'])
        self.assertTrue(config['AttachStdin'])

    def test_run_service_with_links(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['run', 'web', '/bin/true'], None)
        db = self.project.get_service('db')
        console = self.project.get_service('console')
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(console.containers()), 0)

    @v2_only()
    def test_run_service_with_dependencies(self):
        self.base_dir = 'tests/fixtures/v2-dependencies'
        self.dispatch(['run', 'web', '/bin/true'], None)
        db = self.project.get_service('db')
        console = self.project.get_service('console')
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(console.containers()), 0)

    def test_run_service_with_scaled_dependencies(self):
        self.base_dir = 'tests/fixtures/v2-dependencies'
        self.dispatch(['up', '-d', '--scale', 'db=2', '--scale', 'console=0'])
        db = self.project.get_service('db')
        console = self.project.get_service('console')
        assert len(db.containers()) == 2
        assert len(console.containers()) == 0
        self.dispatch(['run', 'web', '/bin/true'], None)
        assert len(db.containers()) == 2
        assert len(console.containers()) == 0

    def test_run_with_no_deps(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['run', '--no-deps', 'web', '/bin/true'])
        db = self.project.get_service('db')
        self.assertEqual(len(db.containers()), 0)

    def test_run_does_not_recreate_linked_containers(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d', 'db'])
        db = self.project.get_service('db')
        self.assertEqual(len(db.containers()), 1)

        old_ids = [c.id for c in db.containers()]

        self.dispatch(['run', 'web', '/bin/true'], None)
        self.assertEqual(len(db.containers()), 1)

        new_ids = [c.id for c in db.containers()]

        self.assertEqual(old_ids, new_ids)

    def test_run_without_command(self):
        self.base_dir = 'tests/fixtures/commands-composefile'
        self.check_build('tests/fixtures/simple-dockerfile', tag='composetest_test')

        self.dispatch(['run', 'implicit'])
        service = self.project.get_service('implicit')
        containers = service.containers(stopped=True, one_off=OneOffFilter.only)
        self.assertEqual(
            [c.human_readable_command for c in containers],
            [u'/bin/sh -c echo "success"'],
        )

        self.dispatch(['run', 'explicit'])
        service = self.project.get_service('explicit')
        containers = service.containers(stopped=True, one_off=OneOffFilter.only)
        self.assertEqual(
            [c.human_readable_command for c in containers],
            [u'/bin/true'],
        )

    def test_run_rm(self):
        self.base_dir = 'tests/fixtures/volume'
        proc = start_process(self.base_dir, ['run', '--rm', 'test'])
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'volume_test_run_1',
            'running'))
        service = self.project.get_service('test')
        containers = service.containers(one_off=OneOffFilter.only)
        self.assertEqual(len(containers), 1)
        mounts = containers[0].get('Mounts')
        for mount in mounts:
            if mount['Destination'] == '/container-path':
                anonymousName = mount['Name']
                break
        os.kill(proc.pid, signal.SIGINT)
        wait_on_process(proc, 1)

        self.assertEqual(len(service.containers(stopped=True, one_off=OneOffFilter.only)), 0)

        volumes = self.client.volumes()['Volumes']
        assert volumes is not None
        for volume in service.options.get('volumes'):
            if volume.internal == '/container-named-path':
                name = volume.external
                break
        volumeNames = [v['Name'] for v in volumes]
        assert name in volumeNames
        assert anonymousName not in volumeNames

    def test_run_service_with_dockerfile_entrypoint(self):
        self.base_dir = 'tests/fixtures/entrypoint-dockerfile'
        self.dispatch(['run', 'test'])
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        assert container.get('Config.Entrypoint') == ['printf']
        assert container.get('Config.Cmd') == ['default', 'args']

    def test_run_service_with_dockerfile_entrypoint_overridden(self):
        self.base_dir = 'tests/fixtures/entrypoint-dockerfile'
        self.dispatch(['run', '--entrypoint', 'echo', 'test'])
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        assert container.get('Config.Entrypoint') == ['echo']
        assert not container.get('Config.Cmd')

    def test_run_service_with_dockerfile_entrypoint_and_command_overridden(self):
        self.base_dir = 'tests/fixtures/entrypoint-dockerfile'
        self.dispatch(['run', '--entrypoint', 'echo', 'test', 'foo'])
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        assert container.get('Config.Entrypoint') == ['echo']
        assert container.get('Config.Cmd') == ['foo']

    def test_run_service_with_compose_file_entrypoint(self):
        self.base_dir = 'tests/fixtures/entrypoint-composefile'
        self.dispatch(['run', 'test'])
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        assert container.get('Config.Entrypoint') == ['printf']
        assert container.get('Config.Cmd') == ['default', 'args']

    def test_run_service_with_compose_file_entrypoint_overridden(self):
        self.base_dir = 'tests/fixtures/entrypoint-composefile'
        self.dispatch(['run', '--entrypoint', 'echo', 'test'])
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        assert container.get('Config.Entrypoint') == ['echo']
        assert not container.get('Config.Cmd')

    def test_run_service_with_compose_file_entrypoint_and_command_overridden(self):
        self.base_dir = 'tests/fixtures/entrypoint-composefile'
        self.dispatch(['run', '--entrypoint', 'echo', 'test', 'foo'])
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        assert container.get('Config.Entrypoint') == ['echo']
        assert container.get('Config.Cmd') == ['foo']

    def test_run_service_with_compose_file_entrypoint_and_empty_string_command(self):
        self.base_dir = 'tests/fixtures/entrypoint-composefile'
        self.dispatch(['run', '--entrypoint', 'echo', 'test', ''])
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        assert container.get('Config.Entrypoint') == ['echo']
        assert container.get('Config.Cmd') == ['']

    def test_run_service_with_user_overridden(self):
        self.base_dir = 'tests/fixtures/user-composefile'
        name = 'service'
        user = 'sshd'
        self.dispatch(['run', '--user={user}'.format(user=user), name], returncode=1)
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=OneOffFilter.only)[0]
        self.assertEqual(user, container.get('Config.User'))

    def test_run_service_with_user_overridden_short_form(self):
        self.base_dir = 'tests/fixtures/user-composefile'
        name = 'service'
        user = 'sshd'
        self.dispatch(['run', '-u', user, name], returncode=1)
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=OneOffFilter.only)[0]
        self.assertEqual(user, container.get('Config.User'))

    def test_run_service_with_environment_overridden(self):
        name = 'service'
        self.base_dir = 'tests/fixtures/environment-composefile'
        self.dispatch([
            'run', '-e', 'foo=notbar',
            '-e', 'allo=moto=bobo',
            '-e', 'alpha=beta',
            name,
            '/bin/true',
        ])
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=OneOffFilter.only)[0]
        # env overridden
        self.assertEqual('notbar', container.environment['foo'])
        # keep environment from yaml
        self.assertEqual('world', container.environment['hello'])
        # added option from command line
        self.assertEqual('beta', container.environment['alpha'])
        # make sure a value with a = don't crash out
        self.assertEqual('moto=bobo', container.environment['allo'])

    def test_run_service_without_map_ports(self):
        # create one off container
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['run', '-d', 'simple'])
        container = self.project.get_service('simple').containers(one_off=OneOffFilter.only)[0]

        # get port information
        port_random = container.get_local_port(3000)
        port_assigned = container.get_local_port(3001)

        # close all one off containers we just created
        container.stop()

        # check the ports
        self.assertEqual(port_random, None)
        self.assertEqual(port_assigned, None)

    def test_run_service_with_map_ports(self):
        # create one off container
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['run', '-d', '--service-ports', 'simple'])
        container = self.project.get_service('simple').containers(one_off=OneOffFilter.only)[0]

        # get port information
        port_random = container.get_local_port(3000)
        port_assigned = container.get_local_port(3001)
        port_range = container.get_local_port(3002), container.get_local_port(3003)

        # close all one off containers we just created
        container.stop()

        # check the ports
        self.assertNotEqual(port_random, None)
        self.assertIn("0.0.0.0", port_random)
        self.assertEqual(port_assigned, "0.0.0.0:49152")
        self.assertEqual(port_range[0], "0.0.0.0:49153")
        self.assertEqual(port_range[1], "0.0.0.0:49154")

    def test_run_service_with_explicitly_mapped_ports(self):
        # create one off container
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['run', '-d', '-p', '30000:3000', '--publish', '30001:3001', 'simple'])
        container = self.project.get_service('simple').containers(one_off=OneOffFilter.only)[0]

        # get port information
        port_short = container.get_local_port(3000)
        port_full = container.get_local_port(3001)

        # close all one off containers we just created
        container.stop()

        # check the ports
        self.assertEqual(port_short, "0.0.0.0:30000")
        self.assertEqual(port_full, "0.0.0.0:30001")

    def test_run_service_with_explicitly_mapped_ip_ports(self):
        # create one off container
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch([
            'run', '-d',
            '-p', '127.0.0.1:30000:3000',
            '--publish', '127.0.0.1:30001:3001',
            'simple'
        ])
        container = self.project.get_service('simple').containers(one_off=OneOffFilter.only)[0]

        # get port information
        port_short = container.get_local_port(3000)
        port_full = container.get_local_port(3001)

        # close all one off containers we just created
        container.stop()

        # check the ports
        self.assertEqual(port_short, "127.0.0.1:30000")
        self.assertEqual(port_full, "127.0.0.1:30001")

    def test_run_with_expose_ports(self):
        # create one off container
        self.base_dir = 'tests/fixtures/expose-composefile'
        self.dispatch(['run', '-d', '--service-ports', 'simple'])
        container = self.project.get_service('simple').containers(one_off=OneOffFilter.only)[0]

        ports = container.ports
        self.assertEqual(len(ports), 9)
        # exposed ports are not mapped to host ports
        assert ports['3000/tcp'] is None
        assert ports['3001/tcp'] is None
        assert ports['3001/udp'] is None
        assert ports['3002/tcp'] is None
        assert ports['3003/tcp'] is None
        assert ports['3004/tcp'] is None
        assert ports['3005/tcp'] is None
        assert ports['3006/udp'] is None
        assert ports['3007/udp'] is None

        # close all one off containers we just created
        container.stop()

    def test_run_with_custom_name(self):
        self.base_dir = 'tests/fixtures/environment-composefile'
        name = 'the-container-name'
        self.dispatch(['run', '--name', name, 'service', '/bin/true'])

        service = self.project.get_service('service')
        container, = service.containers(stopped=True, one_off=OneOffFilter.only)
        self.assertEqual(container.name, name)

    def test_run_service_with_workdir_overridden(self):
        self.base_dir = 'tests/fixtures/run-workdir'
        name = 'service'
        workdir = '/var'
        self.dispatch(['run', '--workdir={workdir}'.format(workdir=workdir), name])
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=True)[0]
        self.assertEqual(workdir, container.get('Config.WorkingDir'))

    def test_run_service_with_workdir_overridden_short_form(self):
        self.base_dir = 'tests/fixtures/run-workdir'
        name = 'service'
        workdir = '/var'
        self.dispatch(['run', '-w', workdir, name])
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=True)[0]
        self.assertEqual(workdir, container.get('Config.WorkingDir'))

    @v2_only()
    def test_run_interactive_connects_to_network(self):
        self.base_dir = 'tests/fixtures/networks'

        self.dispatch(['up', '-d'])
        self.dispatch(['run', 'app', 'nslookup', 'app'])
        self.dispatch(['run', 'app', 'nslookup', 'db'])

        containers = self.project.get_service('app').containers(
            stopped=True, one_off=OneOffFilter.only)
        assert len(containers) == 2

        for container in containers:
            networks = container.get('NetworkSettings.Networks')

            assert sorted(list(networks)) == [
                '{}_{}'.format(self.project.name, name)
                for name in ['back', 'front']
            ]

            for _, config in networks.items():
                # TODO: once we drop support for API <1.24, this can be changed to:
                # assert config['Aliases'] == [container.short_id]
                aliases = set(config['Aliases'] or []) - set([container.short_id])
                assert not aliases

    @v2_only()
    def test_run_detached_connects_to_network(self):
        self.base_dir = 'tests/fixtures/networks'
        self.dispatch(['up', '-d'])
        self.dispatch(['run', '-d', 'app', 'top'])

        container = self.project.get_service('app').containers(one_off=OneOffFilter.only)[0]
        networks = container.get('NetworkSettings.Networks')

        assert sorted(list(networks)) == [
            '{}_{}'.format(self.project.name, name)
            for name in ['back', 'front']
        ]

        for _, config in networks.items():
            # TODO: once we drop support for API <1.24, this can be changed to:
            # assert config['Aliases'] == [container.short_id]
            aliases = set(config['Aliases'] or []) - set([container.short_id])
            assert not aliases

        assert self.lookup(container, 'app')
        assert self.lookup(container, 'db')

    def test_run_handles_sigint(self):
        proc = start_process(self.base_dir, ['run', '-T', 'simple', 'top'])
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simplecomposefile_simple_run_1',
            'running'))

        os.kill(proc.pid, signal.SIGINT)
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simplecomposefile_simple_run_1',
            'exited'))

    def test_run_handles_sigterm(self):
        proc = start_process(self.base_dir, ['run', '-T', 'simple', 'top'])
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simplecomposefile_simple_run_1',
            'running'))

        os.kill(proc.pid, signal.SIGTERM)
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simplecomposefile_simple_run_1',
            'exited'))

    @mock.patch.dict(os.environ)
    def test_run_unicode_env_values_from_system(self):
        value = 'ą, ć, ę, ł, ń, ó, ś, ź, ż'
        if six.PY2:  # os.environ doesn't support unicode values in Py2
            os.environ['BAR'] = value.encode('utf-8')
        else:  # ... and doesn't support byte values in Py3
            os.environ['BAR'] = value
        self.base_dir = 'tests/fixtures/unicode-environment'
        result = self.dispatch(['run', 'simple'])

        if six.PY2:  # Can't retrieve output on Py3. See issue #3670
            assert value == result.stdout.strip()

        container = self.project.containers(one_off=OneOffFilter.only, stopped=True)[0]
        environment = container.get('Config.Env')
        assert 'FOO={}'.format(value) in environment

    @mock.patch.dict(os.environ)
    def test_run_env_values_from_system(self):
        os.environ['FOO'] = 'bar'
        os.environ['BAR'] = 'baz'

        self.dispatch(['run', '-e', 'FOO', 'simple', 'true'], None)

        container = self.project.containers(one_off=OneOffFilter.only, stopped=True)[0]
        environment = container.get('Config.Env')
        assert 'FOO=bar' in environment
        assert 'BAR=baz' not in environment

    def test_rm(self):
        service = self.project.get_service('simple')
        service.create_container()
        kill_service(service)
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.dispatch(['rm', '--force'], None)
        self.assertEqual(len(service.containers(stopped=True)), 0)
        service = self.project.get_service('simple')
        service.create_container()
        kill_service(service)
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.dispatch(['rm', '-f'], None)
        self.assertEqual(len(service.containers(stopped=True)), 0)
        service = self.project.get_service('simple')
        service.create_container()
        self.dispatch(['rm', '-fs'], None)
        self.assertEqual(len(service.containers(stopped=True)), 0)

    def test_rm_stop(self):
        self.dispatch(['up', '-d'], None)
        simple = self.project.get_service('simple')
        another = self.project.get_service('another')
        assert len(simple.containers()) == 1
        assert len(another.containers()) == 1
        self.dispatch(['rm', '-fs'], None)
        assert len(simple.containers(stopped=True)) == 0
        assert len(another.containers(stopped=True)) == 0

        self.dispatch(['up', '-d'], None)
        assert len(simple.containers()) == 1
        assert len(another.containers()) == 1
        self.dispatch(['rm', '-fs', 'another'], None)
        assert len(simple.containers()) == 1
        assert len(another.containers(stopped=True)) == 0

    def test_rm_all(self):
        service = self.project.get_service('simple')
        service.create_container(one_off=False)
        service.create_container(one_off=True)
        kill_service(service)
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertEqual(len(service.containers(stopped=True, one_off=OneOffFilter.only)), 1)
        self.dispatch(['rm', '-f'], None)
        self.assertEqual(len(service.containers(stopped=True)), 0)
        self.assertEqual(len(service.containers(stopped=True, one_off=OneOffFilter.only)), 0)

        service.create_container(one_off=False)
        service.create_container(one_off=True)
        kill_service(service)
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertEqual(len(service.containers(stopped=True, one_off=OneOffFilter.only)), 1)
        self.dispatch(['rm', '-f', '--all'], None)
        self.assertEqual(len(service.containers(stopped=True)), 0)
        self.assertEqual(len(service.containers(stopped=True, one_off=OneOffFilter.only)), 0)

    def test_stop(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)
        self.assertTrue(service.containers()[0].is_running)

        self.dispatch(['stop', '-t', '1'], None)

        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertFalse(service.containers(stopped=True)[0].is_running)

    def test_stop_signal(self):
        self.base_dir = 'tests/fixtures/stop-signal-composefile'
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)
        self.assertTrue(service.containers()[0].is_running)

        self.dispatch(['stop', '-t', '1'], None)
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertFalse(service.containers(stopped=True)[0].is_running)
        self.assertEqual(service.containers(stopped=True)[0].exit_code, 0)

    def test_start_no_containers(self):
        result = self.dispatch(['start'], returncode=1)
        assert 'No containers to start' in result.stderr

    @v2_only()
    def test_up_logging(self):
        self.base_dir = 'tests/fixtures/logging-composefile'
        self.dispatch(['up', '-d'])
        simple = self.project.get_service('simple').containers()[0]
        log_config = simple.get('HostConfig.LogConfig')
        self.assertTrue(log_config)
        self.assertEqual(log_config.get('Type'), 'none')

        another = self.project.get_service('another').containers()[0]
        log_config = another.get('HostConfig.LogConfig')
        self.assertTrue(log_config)
        self.assertEqual(log_config.get('Type'), 'json-file')
        self.assertEqual(log_config.get('Config')['max-size'], '10m')

    def test_up_logging_legacy(self):
        self.base_dir = 'tests/fixtures/logging-composefile-legacy'
        self.dispatch(['up', '-d'])
        simple = self.project.get_service('simple').containers()[0]
        log_config = simple.get('HostConfig.LogConfig')
        self.assertTrue(log_config)
        self.assertEqual(log_config.get('Type'), 'none')

        another = self.project.get_service('another').containers()[0]
        log_config = another.get('HostConfig.LogConfig')
        self.assertTrue(log_config)
        self.assertEqual(log_config.get('Type'), 'json-file')
        self.assertEqual(log_config.get('Config')['max-size'], '10m')

    def test_pause_unpause(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertFalse(service.containers()[0].is_paused)

        self.dispatch(['pause'], None)
        self.assertTrue(service.containers()[0].is_paused)

        self.dispatch(['unpause'], None)
        self.assertFalse(service.containers()[0].is_paused)

    def test_pause_no_containers(self):
        result = self.dispatch(['pause'], returncode=1)
        assert 'No containers to pause' in result.stderr

    def test_unpause_no_containers(self):
        result = self.dispatch(['unpause'], returncode=1)
        assert 'No containers to unpause' in result.stderr

    def test_logs_invalid_service_name(self):
        self.dispatch(['logs', 'madeupname'], returncode=1)

    def test_logs_follow(self):
        self.base_dir = 'tests/fixtures/echo-services'
        self.dispatch(['up', '-d'])

        result = self.dispatch(['logs', '-f'])

        assert result.stdout.count('\n') == 5
        assert 'simple' in result.stdout
        assert 'another' in result.stdout
        assert 'exited with code 0' in result.stdout

    def test_logs_follow_logs_from_new_containers(self):
        self.base_dir = 'tests/fixtures/logs-composefile'
        self.dispatch(['up', '-d', 'simple'])

        proc = start_process(self.base_dir, ['logs', '-f'])

        self.dispatch(['up', '-d', 'another'])
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'logscomposefile_another_1',
            'exited'))

        self.dispatch(['kill', 'simple'])

        result = wait_on_process(proc)

        assert 'hello' in result.stdout
        assert 'test' in result.stdout
        assert 'logscomposefile_another_1 exited with code 0' in result.stdout
        assert 'logscomposefile_simple_1 exited with code 137' in result.stdout

    def test_logs_default(self):
        self.base_dir = 'tests/fixtures/logs-composefile'
        self.dispatch(['up', '-d'])

        result = self.dispatch(['logs'])
        assert 'hello' in result.stdout
        assert 'test' in result.stdout
        assert 'exited with' not in result.stdout

    def test_logs_on_stopped_containers_exits(self):
        self.base_dir = 'tests/fixtures/echo-services'
        self.dispatch(['up'])

        result = self.dispatch(['logs'])
        assert 'simple' in result.stdout
        assert 'another' in result.stdout
        assert 'exited with' not in result.stdout

    def test_logs_timestamps(self):
        self.base_dir = 'tests/fixtures/echo-services'
        self.dispatch(['up', '-d'])

        result = self.dispatch(['logs', '-f', '-t'])
        self.assertRegexpMatches(result.stdout, '(\d{4})-(\d{2})-(\d{2})T(\d{2})\:(\d{2})\:(\d{2})')

    def test_logs_tail(self):
        self.base_dir = 'tests/fixtures/logs-tail-composefile'
        self.dispatch(['up'])

        result = self.dispatch(['logs', '--tail', '2'])
        assert result.stdout.count('\n') == 3

    def test_kill(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)
        self.assertTrue(service.containers()[0].is_running)

        self.dispatch(['kill'], None)

        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertFalse(service.containers(stopped=True)[0].is_running)

    def test_kill_signal_sigstop(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)
        self.assertTrue(service.containers()[0].is_running)

        self.dispatch(['kill', '-s', 'SIGSTOP'], None)

        self.assertEqual(len(service.containers()), 1)
        # The container is still running. It has only been paused
        self.assertTrue(service.containers()[0].is_running)

    def test_kill_stopped_service(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.dispatch(['kill', '-s', 'SIGSTOP'], None)
        self.assertTrue(service.containers()[0].is_running)

        self.dispatch(['kill', '-s', 'SIGKILL'], None)

        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertFalse(service.containers(stopped=True)[0].is_running)

    def test_restart(self):
        service = self.project.get_service('simple')
        container = service.create_container()
        service.start_container(container)
        started_at = container.dictionary['State']['StartedAt']
        self.dispatch(['restart', '-t', '1'], None)
        container.inspect()
        self.assertNotEqual(
            container.dictionary['State']['FinishedAt'],
            '0001-01-01T00:00:00Z',
        )
        self.assertNotEqual(
            container.dictionary['State']['StartedAt'],
            started_at,
        )

    def test_restart_stopped_container(self):
        service = self.project.get_service('simple')
        container = service.create_container()
        container.start()
        container.kill()
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.dispatch(['restart', '-t', '1'], None)
        self.assertEqual(len(service.containers(stopped=False)), 1)

    def test_restart_no_containers(self):
        result = self.dispatch(['restart'], returncode=1)
        assert 'No containers to restart' in result.stderr

    def test_scale(self):
        project = self.project

        self.dispatch(['scale', 'simple=1'])
        self.assertEqual(len(project.get_service('simple').containers()), 1)

        self.dispatch(['scale', 'simple=3', 'another=2'])
        self.assertEqual(len(project.get_service('simple').containers()), 3)
        self.assertEqual(len(project.get_service('another').containers()), 2)

        self.dispatch(['scale', 'simple=1', 'another=1'])
        self.assertEqual(len(project.get_service('simple').containers()), 1)
        self.assertEqual(len(project.get_service('another').containers()), 1)

        self.dispatch(['scale', 'simple=1', 'another=1'])
        self.assertEqual(len(project.get_service('simple').containers()), 1)
        self.assertEqual(len(project.get_service('another').containers()), 1)

        self.dispatch(['scale', 'simple=0', 'another=0'])
        self.assertEqual(len(project.get_service('simple').containers()), 0)
        self.assertEqual(len(project.get_service('another').containers()), 0)

    def test_scale_v2_2(self):
        self.base_dir = 'tests/fixtures/scale'
        result = self.dispatch(['scale', 'web=1'], returncode=1)
        assert 'incompatible with the v2.2 format' in result.stderr

    def test_up_scale_scale_up(self):
        self.base_dir = 'tests/fixtures/scale'
        project = self.project

        self.dispatch(['up', '-d'])
        assert len(project.get_service('web').containers()) == 2
        assert len(project.get_service('db').containers()) == 1

        self.dispatch(['up', '-d', '--scale', 'web=3'])
        assert len(project.get_service('web').containers()) == 3
        assert len(project.get_service('db').containers()) == 1

    def test_up_scale_scale_down(self):
        self.base_dir = 'tests/fixtures/scale'
        project = self.project

        self.dispatch(['up', '-d'])
        assert len(project.get_service('web').containers()) == 2
        assert len(project.get_service('db').containers()) == 1

        self.dispatch(['up', '-d', '--scale', 'web=1'])
        assert len(project.get_service('web').containers()) == 1
        assert len(project.get_service('db').containers()) == 1

    def test_up_scale_reset(self):
        self.base_dir = 'tests/fixtures/scale'
        project = self.project

        self.dispatch(['up', '-d', '--scale', 'web=3', '--scale', 'db=3'])
        assert len(project.get_service('web').containers()) == 3
        assert len(project.get_service('db').containers()) == 3

        self.dispatch(['up', '-d'])
        assert len(project.get_service('web').containers()) == 2
        assert len(project.get_service('db').containers()) == 1

    def test_up_scale_to_zero(self):
        self.base_dir = 'tests/fixtures/scale'
        project = self.project

        self.dispatch(['up', '-d'])
        assert len(project.get_service('web').containers()) == 2
        assert len(project.get_service('db').containers()) == 1

        self.dispatch(['up', '-d', '--scale', 'web=0', '--scale', 'db=0'])
        assert len(project.get_service('web').containers()) == 0
        assert len(project.get_service('db').containers()) == 0

    def test_port(self):
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['up', '-d'], None)
        container = self.project.get_service('simple').get_container()

        def get_port(number):
            result = self.dispatch(['port', 'simple', str(number)])
            return result.stdout.rstrip()

        self.assertEqual(get_port(3000), container.get_local_port(3000))
        self.assertEqual(get_port(3001), "0.0.0.0:49152")
        self.assertEqual(get_port(3002), "0.0.0.0:49153")

    def test_expanded_port(self):
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['-f', 'expanded-notation.yml', 'up', '-d'])
        container = self.project.get_service('simple').get_container()

        def get_port(number):
            result = self.dispatch(['port', 'simple', str(number)])
            return result.stdout.rstrip()

        self.assertEqual(get_port(3000), container.get_local_port(3000))
        self.assertEqual(get_port(3001), "0.0.0.0:49152")
        self.assertEqual(get_port(3002), "0.0.0.0:49153")

    def test_port_with_scale(self):
        self.base_dir = 'tests/fixtures/ports-composefile-scale'
        self.dispatch(['scale', 'simple=2'], None)
        containers = sorted(
            self.project.containers(service_names=['simple']),
            key=attrgetter('name'))

        def get_port(number, index=None):
            if index is None:
                result = self.dispatch(['port', 'simple', str(number)])
            else:
                result = self.dispatch(['port', '--index=' + str(index), 'simple', str(number)])
            return result.stdout.rstrip()

        self.assertEqual(get_port(3000), containers[0].get_local_port(3000))
        self.assertEqual(get_port(3000, index=1), containers[0].get_local_port(3000))
        self.assertEqual(get_port(3000, index=2), containers[1].get_local_port(3000))
        self.assertEqual(get_port(3002), "")

    def test_events_json(self):
        events_proc = start_process(self.base_dir, ['events', '--json'])
        self.dispatch(['up', '-d'])
        wait_on_condition(ContainerCountCondition(self.project, 2))

        os.kill(events_proc.pid, signal.SIGINT)
        result = wait_on_process(events_proc, returncode=1)
        lines = [json.loads(line) for line in result.stdout.rstrip().split('\n')]
        assert Counter(e['action'] for e in lines) == {'create': 2, 'start': 2}

    def test_events_human_readable(self):

        def has_timestamp(string):
            str_iso_date, str_iso_time, container_info = string.split(' ', 2)
            try:
                return isinstance(datetime.datetime.strptime(
                    '%s %s' % (str_iso_date, str_iso_time),
                    '%Y-%m-%d %H:%M:%S.%f'),
                    datetime.datetime)
            except ValueError:
                return False

        events_proc = start_process(self.base_dir, ['events'])
        self.dispatch(['up', '-d', 'simple'])
        wait_on_condition(ContainerCountCondition(self.project, 1))

        os.kill(events_proc.pid, signal.SIGINT)
        result = wait_on_process(events_proc, returncode=1)
        lines = result.stdout.rstrip().split('\n')
        assert len(lines) == 2

        container, = self.project.containers()
        expected_template = (
            ' container {} {} (image=busybox:latest, '
            'name=simplecomposefile_simple_1)')

        assert expected_template.format('create', container.id) in lines[0]
        assert expected_template.format('start', container.id) in lines[1]

        assert has_timestamp(lines[0])

    def test_env_file_relative_to_compose_file(self):
        config_path = os.path.abspath('tests/fixtures/env-file/docker-compose.yml')
        self.dispatch(['-f', config_path, 'up', '-d'], None)
        self._project = get_project(self.base_dir, [config_path])

        containers = self.project.containers(stopped=True)
        self.assertEqual(len(containers), 1)
        self.assertIn("FOO=1", containers[0].get('Config.Env'))

    @mock.patch.dict(os.environ)
    def test_home_and_env_var_in_volume_path(self):
        os.environ['VOLUME_NAME'] = 'my-volume'
        os.environ['HOME'] = '/tmp/home-dir'

        self.base_dir = 'tests/fixtures/volume-path-interpolation'
        self.dispatch(['up', '-d'], None)

        container = self.project.containers(stopped=True)[0]
        actual_host_path = container.get_mount('/container-path')['Source']
        components = actual_host_path.split('/')
        assert components[-2:] == ['home-dir', 'my-volume']

    def test_up_with_default_override_file(self):
        self.base_dir = 'tests/fixtures/override-files'
        self.dispatch(['up', '-d'], None)

        containers = self.project.containers()
        self.assertEqual(len(containers), 2)

        web, db = containers
        self.assertEqual(web.human_readable_command, 'top')
        self.assertEqual(db.human_readable_command, 'top')

    def test_up_with_multiple_files(self):
        self.base_dir = 'tests/fixtures/override-files'
        config_paths = [
            'docker-compose.yml',
            'docker-compose.override.yml',
            'extra.yml',

        ]
        self._project = get_project(self.base_dir, config_paths)
        self.dispatch(
            [
                '-f', config_paths[0],
                '-f', config_paths[1],
                '-f', config_paths[2],
                'up', '-d',
            ],
            None)

        containers = self.project.containers()
        self.assertEqual(len(containers), 3)

        web, other, db = containers
        self.assertEqual(web.human_readable_command, 'top')
        self.assertTrue({'db', 'other'} <= set(get_links(web)))
        self.assertEqual(db.human_readable_command, 'top')
        self.assertEqual(other.human_readable_command, 'top')

    def test_up_with_extends(self):
        self.base_dir = 'tests/fixtures/extends'
        self.dispatch(['up', '-d'], None)

        self.assertEqual(
            set([s.name for s in self.project.services]),
            set(['mydb', 'myweb']),
        )

        # Sort by name so we get [db, web]
        containers = sorted(
            self.project.containers(stopped=True),
            key=lambda c: c.name,
        )

        self.assertEqual(len(containers), 2)
        web = containers[1]

        self.assertEqual(
            set(get_links(web)),
            set(['db', 'mydb_1', 'extends_mydb_1']))

        expected_env = set([
            "FOO=1",
            "BAR=2",
            "BAZ=2",
        ])
        self.assertTrue(expected_env <= set(web.get('Config.Env')))

    def test_top_services_not_running(self):
        self.base_dir = 'tests/fixtures/top'
        result = self.dispatch(['top'])
        assert len(result.stdout) == 0

    def test_top_services_running(self):
        self.base_dir = 'tests/fixtures/top'
        self.dispatch(['up', '-d'])
        result = self.dispatch(['top'])

        self.assertIn('top_service_a', result.stdout)
        self.assertIn('top_service_b', result.stdout)
        self.assertNotIn('top_not_a_service', result.stdout)

    def test_top_processes_running(self):
        self.base_dir = 'tests/fixtures/top'
        self.dispatch(['up', '-d'])
        result = self.dispatch(['top'])
        assert result.stdout.count("top") == 4

    def test_forward_exitval(self):
        self.base_dir = 'tests/fixtures/exit-code-from'
        proc = start_process(
            self.base_dir,
            ['up', '--abort-on-container-exit', '--exit-code-from', 'another'])

        result = wait_on_process(proc, returncode=1)

        assert 'exitcodefrom_another_1 exited with code 1' in result.stdout

    def test_images(self):
        self.project.get_service('simple').create_container()
        result = self.dispatch(['images'])
        assert 'busybox' in result.stdout
        assert 'simplecomposefile_simple_1' in result.stdout

    def test_images_default_composefile(self):
        self.base_dir = 'tests/fixtures/multiple-composefiles'
        self.dispatch(['up', '-d'])
        result = self.dispatch(['images'])

        assert 'busybox' in result.stdout
        assert 'multiplecomposefiles_another_1' in result.stdout
        assert 'multiplecomposefiles_simple_1' in result.stdout

    def test_up_with_override_yaml(self):
        self.base_dir = 'tests/fixtures/override-yaml-files'
        self._project = get_project(self.base_dir, [])
        self.dispatch(
            [
                'up', '-d',
            ],
            None)

        containers = self.project.containers()
        self.assertEqual(len(containers), 2)

        web, db = containers
        self.assertEqual(web.human_readable_command, 'sleep 100')
        self.assertEqual(db.human_readable_command, 'top')

    def test_up_with_duplicate_override_yaml_files(self):
        self.base_dir = 'tests/fixtures/duplicate-override-yaml-files'
        with self.assertRaises(DuplicateOverrideFileFound):
            get_project(self.base_dir, [])
        self.base_dir = None
