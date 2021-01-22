import datetime
import json
import os.path
import re
import signal
import subprocess
import time
from collections import Counter
from collections import namedtuple
from functools import reduce
from operator import attrgetter

import pytest
import yaml
from docker import errors

from .. import mock
from ..helpers import BUSYBOX_IMAGE_WITH_TAG
from ..helpers import create_host_file
from compose.cli.command import get_project
from compose.config.errors import DuplicateOverrideFileFound
from compose.const import COMPOSE_SPEC as VERSION
from compose.const import COMPOSEFILE_V1 as V1
from compose.container import Container
from compose.project import OneOffFilter
from compose.utils import nanoseconds_from_time_seconds
from tests.integration.testcases import DockerClientTestCase
from tests.integration.testcases import get_links
from tests.integration.testcases import is_cluster
from tests.integration.testcases import no_cluster
from tests.integration.testcases import pull_busybox
from tests.integration.testcases import SWARM_SKIP_RM_VOLUMES

DOCKER_COMPOSE_EXECUTABLE = 'docker-compose'

ProcessResult = namedtuple('ProcessResult', 'stdout stderr')


BUILD_CACHE_TEXT = 'Using cache'
BUILD_PULL_TEXT = 'Status: Image is up to date for busybox:1.27.2'
COMPOSE_COMPATIBILITY_DICT = {
    'version': str(VERSION),
    'volumes': {'foo': {'driver': 'default'}},
    'networks': {'bar': {}},
    'services': {
        'foo': {
            'command': '/bin/true',
            'image': 'alpine:3.10.1',
            'scale': 3,
            'restart': 'always:7',
            'mem_limit': '300M',
            'mem_reservation': '100M',
            'cpus': 0.7,
            'volumes': ['foo:/bar:rw'],
            'networks': {'bar': None},
        }
    },
}


def start_process(base_dir, options, executable=None, env=None):
    executable = executable or DOCKER_COMPOSE_EXECUTABLE
    proc = subprocess.Popen(
        [executable] + options,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        cwd=base_dir,
        env=env,
    )
    print("Running process: %s" % proc.pid)
    return proc


def wait_on_process(proc, returncode=0, stdin=None):
    stdout, stderr = proc.communicate(input=stdin)
    if proc.returncode != returncode:
        print("Stderr: {}".format(stderr))
        print("Stdout: {}".format(stdout))
        assert proc.returncode == returncode
    return ProcessResult(stdout.decode('utf-8'), stderr.decode('utf-8'))


def dispatch(base_dir, options,
             project_options=None, returncode=0, stdin=None, executable=None, env=None):
    project_options = project_options or []
    proc = start_process(base_dir, project_options + options, executable=executable, env=env)
    return wait_on_process(proc, returncode=returncode, stdin=stdin)


def wait_on_condition(condition, delay=0.1, timeout=40):
    start_time = time.time()
    while not condition():
        if time.time() - start_time > timeout:
            raise AssertionError("Timeout: %s" % condition)
        time.sleep(delay)


def kill_service(service):
    for container in service.containers():
        if container.is_running:
            container.kill()


class ContainerCountCondition:

    def __init__(self, project, expected):
        self.project = project
        self.expected = expected

    def __call__(self):
        return len([c for c in self.project.containers() if c.is_running]) == self.expected

    def __str__(self):
        return "waiting for counter count == %s" % self.expected


class ContainerStateCondition:

    def __init__(self, client, name, status):
        self.client = client
        self.name = name
        self.status = status

    def __call__(self):
        try:
            if self.name.endswith('*'):
                ctnrs = self.client.containers(all=True, filters={'name': self.name[:-1]})
                if len(ctnrs) > 0:
                    container = self.client.inspect_container(ctnrs[0]['Id'])
                else:
                    return False
            else:
                container = self.client.inspect_container(self.name)
            return container['State']['Status'] == self.status
        except errors.APIError:
            return False

    def __str__(self):
        return "waiting for container to be %s" % self.status


class CLITestCase(DockerClientTestCase):

    def setUp(self):
        super().setUp()
        self.base_dir = 'tests/fixtures/simple-composefile'
        self.override_dir = None

    def tearDown(self):
        if self.base_dir:
            self.project.kill()
            self.project.down(None, True)

            for container in self.project.containers(stopped=True, one_off=OneOffFilter.only):
                container.remove(force=True)
            networks = self.client.networks()
            for n in networks:
                if n['Name'].split('/')[-1].startswith('{}_'.format(self.project.name)):
                    self.client.remove_network(n['Name'])
            volumes = self.client.volumes().get('Volumes') or []
            for v in volumes:
                if v['Name'].split('/')[-1].startswith('{}_'.format(self.project.name)):
                    self.client.remove_volume(v['Name'])
        if hasattr(self, '_project'):
            del self._project

        super().tearDown()

    @property
    def project(self):
        # Hack: allow project to be overridden
        if not hasattr(self, '_project'):
            self._project = get_project(self.base_dir, override_dir=self.override_dir)
        return self._project

    def dispatch(self, options, project_options=None, returncode=0, stdin=None):
        return dispatch(self.base_dir, options, project_options, returncode, stdin)

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
        assert 'Usage: up [options] [--scale SERVICE=NUM...] [--] [SERVICE...]' in result.stdout
        # Prevent tearDown from trying to create a project
        self.base_dir = None

    def test_quiet_build(self):
        self.base_dir = 'tests/fixtures/build-args'
        result = self.dispatch(['build'], None)
        quietResult = self.dispatch(['build', '-q'], None)
        assert result.stdout != ""
        assert quietResult.stdout == ""

    def test_help_nonexistent(self):
        self.base_dir = 'tests/fixtures/no-composefile'
        result = self.dispatch(['help', 'foobar'], returncode=1)
        assert 'No such command' in result.stderr
        self.base_dir = None

    def test_shorthand_host_opt(self):
        self.dispatch(
            ['-H={}'.format(os.environ.get('DOCKER_HOST', 'unix://')),
             'up', '-d'],
            returncode=0
        )

    def test_shorthand_host_opt_interactive(self):
        self.dispatch(
            ['-H={}'.format(os.environ.get('DOCKER_HOST', 'unix://')),
             'run', 'another', 'ls'],
            returncode=0
        )

    def test_host_not_reachable(self):
        result = self.dispatch(['-H=tcp://doesnotexist:8000', 'ps'], returncode=1)
        assert "Couldn't connect to Docker daemon" in result.stderr

    def test_host_not_reachable_volumes_from_container(self):
        self.base_dir = 'tests/fixtures/volumes-from-container'

        container = self.client.create_container(
            'busybox', 'true', name='composetest_data_container',
            host_config={}
        )
        self.addCleanup(self.client.remove_container, container)

        result = self.dispatch(['-H=tcp://doesnotexist:8000', 'ps'], returncode=1)
        assert "Couldn't connect to Docker daemon" in result.stderr

    def test_config_list_profiles(self):
        self.base_dir = 'tests/fixtures/config-profiles'
        result = self.dispatch(['config', '--profiles'])
        assert set(result.stdout.rstrip().split('\n')) == {'debug', 'frontend', 'gui'}

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
            'config', '--quiet'
        ], returncode=1)
        assert "'notaservice' must be a mapping" in result.stderr

    def test_config_quiet(self):
        self.base_dir = 'tests/fixtures/v2-full'
        assert self.dispatch(['config', '--quiet']).stdout == ''

    def test_config_stdin(self):
        config = b"""version: "3.7"
services:
  web:
    image: nginx
  other:
    image: alpine
"""
        result = self.dispatch(['-f', '-', 'config', '--services'], stdin=config)
        assert set(result.stdout.rstrip().split('\n')) == {'web', 'other'}

    def test_config_with_hash_option(self):
        self.base_dir = 'tests/fixtures/v2-full'
        result = self.dispatch(['config', '--hash=*'])
        for service in self.project.get_services():
            assert '{} {}\n'.format(service.name, service.config_hash) in result.stdout

        svc = self.project.get_service('other')
        result = self.dispatch(['config', '--hash=other'])
        assert result.stdout == '{} {}\n'.format(svc.name, svc.config_hash)

    def test_config_default(self):
        self.base_dir = 'tests/fixtures/v2-full'
        result = self.dispatch(['config'])
        # assert there are no python objects encoded in the output
        assert '!!' not in result.stdout

        output = yaml.safe_load(result.stdout)
        expected = {
            'version': '2',
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
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'top',
                    'volumes': ['/data'],
                },
            },
        }
        assert output == expected

    def test_config_restart(self):
        self.base_dir = 'tests/fixtures/restart'
        result = self.dispatch(['config'])
        assert yaml.safe_load(result.stdout) == {
            'version': '2',
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
        json_result = yaml.safe_load(result.stdout)
        assert 'networks' in json_result
        assert json_result['networks'] == {
            'networks_foo': {
                'external': True,
                'name': 'networks_foo'
            },
            'bar': {
                'external': True,
                'name': 'networks_bar'
            }
        }

    def test_config_with_dot_env(self):
        self.base_dir = 'tests/fixtures/default-env-file'
        result = self.dispatch(['config'])
        json_result = yaml.safe_load(result.stdout)
        assert json_result == {
            'version': '2.4',
            'services': {
                'web': {
                    'command': 'true',
                    'image': 'alpine:latest',
                    'ports': [{'target': 5643}, {'target': 9999}]
                }
            }
        }

    def test_config_with_env_file(self):
        self.base_dir = 'tests/fixtures/default-env-file'
        result = self.dispatch(['--env-file', '.env2', 'config'])
        json_result = yaml.safe_load(result.stdout)
        assert json_result == {
            'version': '2.4',
            'services': {
                'web': {
                    'command': 'false',
                    'image': 'alpine:latest',
                    'ports': [{'target': 5644}, {'target': 9998}]
                }
            }
        }

    def test_config_with_dot_env_and_override_dir(self):
        self.base_dir = 'tests/fixtures/default-env-file'
        result = self.dispatch(['--project-directory', 'alt/', 'config'])
        json_result = yaml.safe_load(result.stdout)
        assert json_result == {
            'version': '2.4',
            'services': {
                'web': {
                    'command': 'echo uwu',
                    'image': 'alpine:3.10.1',
                    'ports': [{'target': 3341}, {'target': 4449}]
                }
            }
        }

    def test_config_external_volume_v2(self):
        self.base_dir = 'tests/fixtures/volumes'
        result = self.dispatch(['-f', 'external-volumes-v2.yml', 'config'])
        json_result = yaml.safe_load(result.stdout)
        assert 'volumes' in json_result
        assert json_result['volumes'] == {
            'foo': {
                'external': True,
                'name': 'foo',
            },
            'bar': {
                'external': True,
                'name': 'some_bar',
            }
        }

    def test_config_external_volume_v2_x(self):
        self.base_dir = 'tests/fixtures/volumes'
        result = self.dispatch(['-f', 'external-volumes-v2-x.yml', 'config'])
        json_result = yaml.safe_load(result.stdout)
        assert 'volumes' in json_result
        assert json_result['volumes'] == {
            'foo': {
                'external': True,
                'name': 'some_foo',
            },
            'bar': {
                'external': True,
                'name': 'some_bar',
            }
        }

    def test_config_external_volume_v3_x(self):
        self.base_dir = 'tests/fixtures/volumes'
        result = self.dispatch(['-f', 'external-volumes-v3-x.yml', 'config'])
        json_result = yaml.safe_load(result.stdout)
        assert 'volumes' in json_result
        assert json_result['volumes'] == {
            'foo': {
                'external': True,
                'name': 'foo',
            },
            'bar': {
                'external': True,
                'name': 'some_bar',
            }
        }

    def test_config_external_volume_v3_4(self):
        self.base_dir = 'tests/fixtures/volumes'
        result = self.dispatch(['-f', 'external-volumes-v3-4.yml', 'config'])
        json_result = yaml.safe_load(result.stdout)
        assert 'volumes' in json_result
        assert json_result['volumes'] == {
            'foo': {
                'external': True,
                'name': 'some_foo',
            },
            'bar': {
                'external': True,
                'name': 'some_bar',
            }
        }

    def test_config_external_network_v3_5(self):
        self.base_dir = 'tests/fixtures/networks'
        result = self.dispatch(['-f', 'external-networks-v3-5.yml', 'config'])
        json_result = yaml.safe_load(result.stdout)
        assert 'networks' in json_result
        assert json_result['networks'] == {
            'foo': {
                'external': True,
                'name': 'some_foo',
            },
            'bar': {
                'external': True,
                'name': 'some_bar',
            },
        }

    def test_config_v1(self):
        self.base_dir = 'tests/fixtures/v1-config'
        result = self.dispatch(['config'])
        assert yaml.safe_load(result.stdout) == {
            'version': str(V1),
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

    def test_config_v3(self):
        self.base_dir = 'tests/fixtures/v3-full'
        result = self.dispatch(['config'])
        assert yaml.safe_load(result.stdout) == {
            'version': '3.5',
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
                                'cpus': 0.05,
                                'memory': '50M',
                            },
                            'reservations': {
                                'cpus': 0.01,
                                'memory': '20M',
                            },
                        },
                        'restart_policy': {
                            'condition': 'on-failure',
                            'delay': '5s',
                            'max_attempts': 3,
                            'window': '120s',
                        },
                        'placement': {
                            'constraints': [
                                'node.hostname==foo', 'node.role != manager'
                            ],
                            'preferences': [{'spread': 'node.labels.datacenter'}]
                        },
                    },

                    'healthcheck': {
                        'test': 'cat /etc/passwd',
                        'interval': '10s',
                        'timeout': '1s',
                        'retries': 5,
                    },
                    'volumes': [{
                        'read_only': True,
                        'source': '/host/path',
                        'target': '/container/path',
                        'type': 'bind'
                    }, {
                        'source': 'foobar', 'target': '/container/volumepath', 'type': 'volume'
                    }, {
                        'target': '/anonymous', 'type': 'volume'
                    }, {
                        'source': 'foobar',
                        'target': '/container/volumepath2',
                        'type': 'volume',
                        'volume': {'nocopy': True}
                    }],
                    'stop_grace_period': '20s',
                },
            },
        }

    @pytest.mark.skip(reason='deprecated option')
    def test_config_compatibility_mode(self):
        self.base_dir = 'tests/fixtures/compatibility-mode'
        result = self.dispatch(['--compatibility', 'config'])

        assert yaml.load(result.stdout) == COMPOSE_COMPATIBILITY_DICT

    @pytest.mark.skip(reason='deprecated option')
    @mock.patch.dict(os.environ)
    def test_config_compatibility_mode_from_env(self):
        self.base_dir = 'tests/fixtures/compatibility-mode'
        os.environ['COMPOSE_COMPATIBILITY'] = 'true'
        result = self.dispatch(['config'])

        assert yaml.load(result.stdout) == COMPOSE_COMPATIBILITY_DICT

    @pytest.mark.skip(reason='deprecated option')
    @mock.patch.dict(os.environ)
    def test_config_compatibility_mode_from_env_and_option_precedence(self):
        self.base_dir = 'tests/fixtures/compatibility-mode'
        os.environ['COMPOSE_COMPATIBILITY'] = 'false'
        result = self.dispatch(['--compatibility', 'config'])

        assert yaml.load(result.stdout) == COMPOSE_COMPATIBILITY_DICT

    def test_ps(self):
        self.project.get_service('simple').create_container()
        result = self.dispatch(['ps'])
        assert 'simple-composefile_simple_1' in result.stdout

    def test_ps_default_composefile(self):
        self.base_dir = 'tests/fixtures/multiple-composefiles'
        self.dispatch(['up', '-d'])
        result = self.dispatch(['ps'])

        assert 'multiple-composefiles_simple_1' in result.stdout
        assert 'multiple-composefiles_another_1' in result.stdout
        assert 'multiple-composefiles_yetanother_1' not in result.stdout

    def test_ps_alternate_composefile(self):
        config_path = os.path.abspath(
            'tests/fixtures/multiple-composefiles/compose2.yml')
        self._project = get_project(self.base_dir, [config_path])

        self.base_dir = 'tests/fixtures/multiple-composefiles'
        self.dispatch(['-f', 'compose2.yml', 'up', '-d'])
        result = self.dispatch(['-f', 'compose2.yml', 'ps'])

        assert 'multiple-composefiles_simple_1' not in result.stdout
        assert 'multiple-composefiles_another_1' not in result.stdout
        assert 'multiple-composefiles_yetanother_1' in result.stdout

    def test_ps_services_filter_option(self):
        self.base_dir = 'tests/fixtures/ps-services-filter'
        image = self.dispatch(['ps', '--services', '--filter', 'source=image'])
        build = self.dispatch(['ps', '--services', '--filter', 'source=build'])
        all_services = self.dispatch(['ps', '--services'])

        assert 'with_build' in all_services.stdout
        assert 'with_image' in all_services.stdout
        assert 'with_build' in build.stdout
        assert 'with_build' not in image.stdout
        assert 'with_image' in image.stdout
        assert 'with_image' not in build.stdout

    def test_ps_services_filter_status(self):
        self.base_dir = 'tests/fixtures/ps-services-filter'
        self.dispatch(['up', '-d'])
        self.dispatch(['pause', 'with_image'])
        paused = self.dispatch(['ps', '--services', '--filter', 'status=paused'])
        stopped = self.dispatch(['ps', '--services', '--filter', 'status=stopped'])
        running = self.dispatch(['ps', '--services', '--filter', 'status=running'])

        assert 'with_build' not in stopped.stdout
        assert 'with_image' not in stopped.stdout
        assert 'with_build' not in paused.stdout
        assert 'with_image' in paused.stdout
        assert 'with_build' in running.stdout
        assert 'with_image' in running.stdout

    def test_ps_all(self):
        self.project.get_service('simple').create_container(one_off='blahblah')
        result = self.dispatch(['ps'])
        assert 'simple-composefile_simple_run_' not in result.stdout

        result2 = self.dispatch(['ps', '--all'])
        assert 'simple-composefile_simple_run_' in result2.stdout

    def test_pull(self):
        result = self.dispatch(['pull'])
        assert 'Pulling simple' in result.stderr
        assert 'Pulling another' in result.stderr
        assert 'done' in result.stderr
        assert 'failed' not in result.stderr

    def test_pull_with_digest(self):
        result = self.dispatch(['-f', 'digest.yml', 'pull', '--no-parallel'])

        assert 'Pulling simple ({})...'.format(BUSYBOX_IMAGE_WITH_TAG) in result.stderr
        assert ('Pulling digest (busybox@'
                'sha256:38a203e1986cf79639cfb9b2e1d6e773de84002feea2d4eb006b520'
                '04ee8502d)...') in result.stderr

    def test_pull_with_ignore_pull_failures(self):
        result = self.dispatch([
            '-f', 'ignore-pull-failures.yml',
            'pull', '--ignore-pull-failures', '--no-parallel']
        )

        assert 'Pulling simple ({})...'.format(BUSYBOX_IMAGE_WITH_TAG) in result.stderr
        assert 'Pulling another (nonexisting-image:latest)...' in result.stderr
        assert ('repository nonexisting-image not found' in result.stderr or
                'image library/nonexisting-image:latest not found' in result.stderr or
                'pull access denied for nonexisting-image' in result.stderr)

    def test_pull_with_quiet(self):
        assert self.dispatch(['pull', '--quiet']).stderr == ''
        assert self.dispatch(['pull', '--quiet']).stdout == ''

    def test_pull_with_parallel_failure(self):
        result = self.dispatch([
            '-f', 'ignore-pull-failures.yml', 'pull'],
            returncode=1
        )

        assert re.search(re.compile('^Pulling simple', re.MULTILINE), result.stderr)
        assert re.search(re.compile('^Pulling another', re.MULTILINE), result.stderr)
        assert re.search(
            re.compile('^ERROR: for another .*does not exist.*', re.MULTILINE),
            result.stderr
        )
        assert re.search(
            re.compile('''^(ERROR: )?(b')?.* nonexisting-image''', re.MULTILINE),
            result.stderr
        )

    def test_pull_can_build(self):
        result = self.dispatch([
            '-f', 'can-build-pull-failures.yml', 'pull'],
            returncode=0
        )
        assert 'Some service image(s) must be built from source' in result.stderr
        assert 'docker-compose build can_build' in result.stderr

    def test_pull_with_no_deps(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        result = self.dispatch(['pull', '--no-parallel', 'web'])
        assert sorted(result.stderr.split('\n'))[1:] == [
            'Pulling web (busybox:1.27.2)...',
        ]

    def test_pull_with_include_deps(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        result = self.dispatch(['pull', '--no-parallel', '--include-deps', 'web'])
        assert sorted(result.stderr.split('\n'))[1:] == [
            'Pulling db (busybox:1.27.2)...',
            'Pulling web (busybox:1.27.2)...',
        ]

    def test_build_plain(self):
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', 'simple'])

        result = self.dispatch(['build', 'simple'])
        assert BUILD_PULL_TEXT not in result.stdout

    def test_build_no_cache(self):
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', 'simple'])

        result = self.dispatch(['build', '--no-cache', 'simple'])
        assert BUILD_CACHE_TEXT not in result.stdout
        assert BUILD_PULL_TEXT not in result.stdout

    def test_up_ignore_missing_build_directory(self):
        self.base_dir = 'tests/fixtures/no-build'
        result = self.dispatch(['up', '--no-build'])

        assert 'alpine exited with code 0' in result.stdout
        self.base_dir = None

    def test_pull_ignore_missing_build_directory(self):
        self.base_dir = 'tests/fixtures/no-build'
        result = self.dispatch(['pull'])

        assert 'Pulling my-alpine' in result.stderr
        self.base_dir = None

    def test_build_pull(self):
        # Make sure we have the latest busybox already
        pull_busybox(self.client)
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', 'simple'], None)

        result = self.dispatch(['build', '--pull', 'simple'])
        if not is_cluster(self.client):
            # If previous build happened on another node, cache won't be available
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

    @mock.patch.dict(os.environ)
    def test_build_log_level(self):
        os.environ['COMPOSE_DOCKER_CLI_BUILD'] = '0'
        os.environ['DOCKER_BUILDKIT'] = '0'
        self.test_env_file_relative_to_compose_file()
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        result = self.dispatch(['--log-level', 'warning', 'build', 'simple'])
        assert result.stderr == ''
        result = self.dispatch(['--log-level', 'debug', 'build', 'simple'])
        assert 'Building simple' in result.stderr
        assert 'Using configuration file' in result.stderr
        self.base_dir = 'tests/fixtures/simple-failing-dockerfile'
        result = self.dispatch(['--log-level', 'critical', 'build', 'simple'], returncode=1)
        assert result.stderr == ''
        result = self.dispatch(['--log-level', 'debug', 'build', 'simple'], returncode=1)
        assert 'Building simple' in result.stderr
        assert 'non-zero code' in result.stderr

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

    @pytest.mark.xfail(True, reason='Flaky on local')
    def test_build_rm(self):
        containers = [
            Container.from_ps(self.project.client, c)
            for c in self.project.client.containers(all=True)
        ]

        assert not containers

        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', '--no-rm', 'simple'], returncode=0)

        containers = [
            Container.from_ps(self.project.client, c)
            for c in self.project.client.containers(all=True)
        ]
        assert containers

        for c in self.project.client.containers(all=True):
            self.addCleanup(self.project.client.remove_container, c, force=True)

    @mock.patch.dict(os.environ)
    def test_build_shm_size_build_option(self):
        os.environ['COMPOSE_DOCKER_CLI_BUILD'] = '0'
        pull_busybox(self.client)
        self.base_dir = 'tests/fixtures/build-shm-size'
        result = self.dispatch(['build', '--no-cache'], None)
        assert 'shm_size: 96' in result.stdout

    @mock.patch.dict(os.environ)
    def test_build_memory_build_option(self):
        os.environ['COMPOSE_DOCKER_CLI_BUILD'] = '0'
        pull_busybox(self.client)
        self.base_dir = 'tests/fixtures/build-memory'
        result = self.dispatch(['build', '--no-cache', '--memory', '96m', 'service'], None)
        assert 'memory: 100663296' in result.stdout  # 96 * 1024 * 1024

    def test_build_with_buildarg_from_compose_file(self):
        pull_busybox(self.client)
        self.base_dir = 'tests/fixtures/build-args'
        result = self.dispatch(['build'], None)
        assert 'Favorite Touhou Character: mariya.kirisame' in result.stdout

    def test_build_with_buildarg_cli_override(self):
        pull_busybox(self.client)
        self.base_dir = 'tests/fixtures/build-args'
        result = self.dispatch(['build', '--build-arg', 'favorite_th_character=sakuya.izayoi'], None)
        assert 'Favorite Touhou Character: sakuya.izayoi' in result.stdout

    @mock.patch.dict(os.environ)
    def test_build_with_buildarg_old_api_version(self):
        pull_busybox(self.client)
        self.base_dir = 'tests/fixtures/build-args'
        os.environ['COMPOSE_API_VERSION'] = '1.24'
        result = self.dispatch(
            ['build', '--build-arg', 'favorite_th_character=reimu.hakurei'], None, returncode=1
        )
        assert '--build-arg is only supported when services are specified' in result.stderr

        result = self.dispatch(
            ['build', '--build-arg', 'favorite_th_character=hong.meiling', 'web'], None
        )
        assert 'Favorite Touhou Character: hong.meiling' in result.stdout

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

    def test_build_parallel(self):
        self.base_dir = 'tests/fixtures/build-multiple-composefile'
        result = self.dispatch(['build', '--parallel'])
        assert 'Successfully tagged build-multiple-composefile_a:latest' in result.stdout
        assert 'Successfully tagged build-multiple-composefile_b:latest' in result.stdout
        assert 'Successfully built' in result.stdout

    def test_create(self):
        self.dispatch(['create'])
        service = self.project.get_service('simple')
        another = self.project.get_service('another')
        service_containers = service.containers(stopped=True)
        another_containers = another.containers(stopped=True)
        assert len(service_containers) == 1
        assert len(another_containers) == 1
        assert not service_containers[0].is_running
        assert not another_containers[0].is_running

    def test_create_with_force_recreate(self):
        self.dispatch(['create'], None)
        service = self.project.get_service('simple')
        service_containers = service.containers(stopped=True)
        assert len(service_containers) == 1
        assert not service_containers[0].is_running

        old_ids = [c.id for c in service.containers(stopped=True)]

        self.dispatch(['create', '--force-recreate'], None)
        service_containers = service.containers(stopped=True)
        assert len(service_containers) == 1
        assert not service_containers[0].is_running

        new_ids = [c.id for c in service_containers]

        assert old_ids != new_ids

    def test_create_with_no_recreate(self):
        self.dispatch(['create'], None)
        service = self.project.get_service('simple')
        service_containers = service.containers(stopped=True)
        assert len(service_containers) == 1
        assert not service_containers[0].is_running

        old_ids = [c.id for c in service.containers(stopped=True)]

        self.dispatch(['create', '--no-recreate'], None)
        service_containers = service.containers(stopped=True)
        assert len(service_containers) == 1
        assert not service_containers[0].is_running

        new_ids = [c.id for c in service_containers]

        assert old_ids == new_ids

    def test_run_one_off_with_volume(self):
        self.base_dir = 'tests/fixtures/simple-composefile-volume-ready'
        volume_path = os.path.abspath(os.path.join(os.getcwd(), self.base_dir, 'files'))
        node = create_host_file(self.client, os.path.join(volume_path, 'example.txt'))

        self.dispatch([
            'run',
            '-v', '{}:/data'.format(volume_path),
            '-e', 'constraint:node=={}'.format(node if node is not None else '*'),
            'simple',
            'test', '-f', '/data/example.txt'
        ], returncode=0)

        service = self.project.get_service('simple')
        container_data = service.containers(one_off=OneOffFilter.only, stopped=True)[0]
        mount = container_data.get('Mounts')[0]
        assert mount['Source'] == volume_path
        assert mount['Destination'] == '/data'
        assert mount['Type'] == 'bind'

    def test_run_one_off_with_multiple_volumes(self):
        self.base_dir = 'tests/fixtures/simple-composefile-volume-ready'
        volume_path = os.path.abspath(os.path.join(os.getcwd(), self.base_dir, 'files'))
        node = create_host_file(self.client, os.path.join(volume_path, 'example.txt'))

        self.dispatch([
            'run',
            '-v', '{}:/data'.format(volume_path),
            '-v', '{}:/data1'.format(volume_path),
            '-e', 'constraint:node=={}'.format(node if node is not None else '*'),
            'simple',
            'test', '-f', '/data/example.txt'
        ], returncode=0)

        self.dispatch([
            'run',
            '-v', '{}:/data'.format(volume_path),
            '-v', '{}:/data1'.format(volume_path),
            '-e', 'constraint:node=={}'.format(node if node is not None else '*'),
            'simple',
            'test', '-f' '/data1/example.txt'
        ], returncode=0)

    def test_run_one_off_with_volume_merge(self):
        self.base_dir = 'tests/fixtures/simple-composefile-volume-ready'
        volume_path = os.path.abspath(os.path.join(os.getcwd(), self.base_dir, 'files'))
        node = create_host_file(self.client, os.path.join(volume_path, 'example.txt'))

        self.dispatch([
            '-f', 'docker-compose.merge.yml',
            'run',
            '-v', '{}:/data'.format(volume_path),
            '-e', 'constraint:node=={}'.format(node if node is not None else '*'),
            'simple',
            'test', '-f', '/data/example.txt'
        ], returncode=0)

        service = self.project.get_service('simple')
        container_data = service.containers(one_off=OneOffFilter.only, stopped=True)[0]
        mounts = container_data.get('Mounts')
        assert len(mounts) == 2
        config_mount = [m for m in mounts if m['Destination'] == '/data1'][0]
        override_mount = [m for m in mounts if m['Destination'] == '/data'][0]

        assert config_mount['Type'] == 'volume'
        assert override_mount['Source'] == volume_path
        assert override_mount['Type'] == 'bind'

    def test_create_with_force_recreate_and_no_recreate(self):
        self.dispatch(
            ['create', '--force-recreate', '--no-recreate'],
            returncode=1)

    def test_down_invalid_rmi_flag(self):
        result = self.dispatch(['down', '--rmi', 'bogus'], returncode=1)
        assert '--rmi flag must be' in result.stderr

    def test_down(self):
        self.base_dir = 'tests/fixtures/v2-full'

        self.dispatch(['up', '-d'])
        wait_on_condition(ContainerCountCondition(self.project, 2))

        self.dispatch(['run', 'web', 'true'])
        self.dispatch(['run', '-d', 'web', 'tail', '-f', '/dev/null'])
        assert len(self.project.containers(one_off=OneOffFilter.only, stopped=True)) == 2

        result = self.dispatch(['down', '--rmi=local', '--volumes'])
        assert 'Stopping v2-full_web_1' in result.stderr
        assert 'Stopping v2-full_other_1' in result.stderr
        assert 'Stopping v2-full_web_run_' in result.stderr
        assert 'Removing v2-full_web_1' in result.stderr
        assert 'Removing v2-full_other_1' in result.stderr
        assert 'Removing v2-full_web_run_' in result.stderr
        assert 'Removing v2-full_web_run_' in result.stderr
        assert 'Removing volume v2-full_data' in result.stderr
        assert 'Removing image v2-full_web' in result.stderr
        assert 'Removing image busybox' not in result.stderr
        assert 'Removing network v2-full_default' in result.stderr
        assert 'Removing network v2-full_front' in result.stderr

    def test_down_timeout(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        assert len(service.containers()) == 1
        assert service.containers()[0].is_running
        ""

        self.dispatch(['down', '-t', '1'], None)

        assert len(service.containers(stopped=True)) == 0

    def test_down_signal(self):
        self.base_dir = 'tests/fixtures/stop-signal-composefile'
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        assert len(service.containers()) == 1
        assert service.containers()[0].is_running

        self.dispatch(['down', '-t', '1'], None)
        assert len(service.containers(stopped=True)) == 0

    def test_up_detached(self):
        self.dispatch(['up', '-d'])
        service = self.project.get_service('simple')
        another = self.project.get_service('another')
        assert len(service.containers()) == 1
        assert len(another.containers()) == 1

        # Ensure containers don't have stdin and stdout connected in -d mode
        container, = service.containers()
        assert not container.get('Config.AttachStderr')
        assert not container.get('Config.AttachStdout')
        assert not container.get('Config.AttachStdin')

    def test_up_detached_long_form(self):
        self.dispatch(['up', '--detach'])
        service = self.project.get_service('simple')
        another = self.project.get_service('another')
        assert len(service.containers()) == 1
        assert len(another.containers()) == 1

        # Ensure containers don't have stdin and stdout connected in -d mode
        container, = service.containers()
        assert not container.get('Config.AttachStderr')
        assert not container.get('Config.AttachStdout')
        assert not container.get('Config.AttachStdin')

    def test_up_attached(self):
        self.base_dir = 'tests/fixtures/echo-services'
        result = self.dispatch(['up', '--no-color'])
        simple_name = self.project.get_service('simple').containers(stopped=True)[0].name_without_project
        another_name = self.project.get_service('another').containers(
            stopped=True
        )[0].name_without_project

        assert '{}   | simple'.format(simple_name) in result.stdout
        assert '{}  | another'.format(another_name) in result.stdout
        assert '{} exited with code 0'.format(simple_name) in result.stdout
        assert '{} exited with code 0'.format(another_name) in result.stdout

    def test_up(self):
        self.base_dir = 'tests/fixtures/v2-simple'
        self.dispatch(['up', '-d'], None)

        services = self.project.get_services()

        network_name = self.project.networks.networks['default'].full_name
        networks = self.client.networks(names=[network_name])
        assert len(networks) == 1
        assert networks[0]['Driver'] == 'bridge' if not is_cluster(self.client) else 'overlay'
        assert 'com.docker.network.bridge.enable_icc' not in networks[0]['Options']

        network = self.client.inspect_network(networks[0]['Id'])

        for service in services:
            containers = service.containers()
            assert len(containers) == 1

            container = containers[0]
            assert container.id in network['Containers']

            networks = container.get('NetworkSettings.Networks')
            assert list(networks) == [network['Name']]

            assert sorted(networks[network['Name']]['Aliases']) == sorted(
                [service.name, container.short_id]
            )

            for service in services:
                assert self.lookup(container, service.name)

    def test_up_no_start(self):
        self.base_dir = 'tests/fixtures/v2-full'
        self.dispatch(['up', '--no-start'], None)

        services = self.project.get_services()

        default_network = self.project.networks.networks['default'].full_name
        front_network = self.project.networks.networks['front'].full_name
        networks = self.client.networks(names=[default_network, front_network])
        assert len(networks) == 2

        for service in services:
            containers = service.containers(stopped=True)
            assert len(containers) == 1

            container = containers[0]
            assert not container.is_running
            assert container.get('State.Status') == 'created'

        volumes = self.project.volumes.volumes
        assert 'data' in volumes
        volume = volumes['data']

        # The code below is a Swarm-compatible equivalent to volume.exists()
        remote_volumes = [
            v for v in self.client.volumes().get('Volumes', [])
            if v['Name'].split('/')[-1] == volume.full_name
        ]
        assert len(remote_volumes) > 0

    def test_up_no_start_remove_orphans(self):
        self.base_dir = 'tests/fixtures/v2-simple'
        self.dispatch(['up', '--no-start'], None)

        services = self.project.get_services()

        stopped = reduce((lambda prev, next: prev.containers(
            stopped=True) + next.containers(stopped=True)), services)
        assert len(stopped) == 2

        self.dispatch(['-f', 'one-container.yml', 'up', '--no-start', '--remove-orphans'], None)
        stopped2 = reduce((lambda prev, next: prev.containers(
            stopped=True) + next.containers(stopped=True)), services)
        assert len(stopped2) == 1

    def test_up_no_ansi(self):
        self.base_dir = 'tests/fixtures/v2-simple'
        result = self.dispatch(['--no-ansi', 'up', '-d'], None)
        assert "%c[2K\r" % 27 not in result.stderr
        assert "%c[1A" % 27 not in result.stderr
        assert "%c[1B" % 27 not in result.stderr

    def test_up_with_default_network_config(self):
        filename = 'default-network-config.yml'

        self.base_dir = 'tests/fixtures/networks'
        self._project = get_project(self.base_dir, [filename])

        self.dispatch(['-f', filename, 'up', '-d'], None)

        network_name = self.project.networks.networks['default'].full_name
        networks = self.client.networks(names=[network_name])

        assert networks[0]['Options']['com.docker.network.bridge.enable_icc'] == 'false'

    def test_up_with_network_aliases(self):
        filename = 'network-aliases.yml'
        self.base_dir = 'tests/fixtures/networks'
        self.dispatch(['-f', filename, 'up', '-d'], None)
        back_name = '{}_back'.format(self.project.name)
        front_name = '{}_front'.format(self.project.name)

        networks = [
            n for n in self.client.networks()
            if n['Name'].split('/')[-1].startswith('{}_'.format(self.project.name))
        ]

        # Two networks were created: back and front
        assert sorted(n['Name'].split('/')[-1] for n in networks) == [back_name, front_name]
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

    def test_up_with_network_internal(self):
        self.require_api_version('1.23')
        filename = 'network-internal.yml'
        self.base_dir = 'tests/fixtures/networks'
        self.dispatch(['-f', filename, 'up', '-d'], None)
        internal_net = '{}_internal'.format(self.project.name)

        networks = [
            n for n in self.client.networks()
            if n['Name'].split('/')[-1].startswith('{}_'.format(self.project.name))
        ]

        # One network was created: internal
        assert sorted(n['Name'].split('/')[-1] for n in networks) == [internal_net]

        assert networks[0]['Internal'] is True

    def test_up_with_network_static_addresses(self):
        filename = 'network-static-addresses.yml'
        ipv4_address = '172.16.100.100'
        ipv6_address = 'fe80::1001:100'
        self.base_dir = 'tests/fixtures/networks'
        self.dispatch(['-f', filename, 'up', '-d'], None)
        static_net = '{}_static_test'.format(self.project.name)

        networks = [
            n for n in self.client.networks()
            if n['Name'].split('/')[-1].startswith('{}_'.format(self.project.name))
        ]

        # One networks was created: front
        assert sorted(n['Name'].split('/')[-1] for n in networks) == [static_net]
        web_container = self.project.get_service('web').containers()[0]

        ipam_config = web_container.get(
            'NetworkSettings.Networks.{}.IPAMConfig'.format(static_net)
        )
        assert ipv4_address in ipam_config.values()
        assert ipv6_address in ipam_config.values()

    def test_up_with_networks(self):
        self.base_dir = 'tests/fixtures/networks'
        self.dispatch(['up', '-d'], None)

        back_name = '{}_back'.format(self.project.name)
        front_name = '{}_front'.format(self.project.name)

        networks = [
            n for n in self.client.networks()
            if n['Name'].split('/')[-1].startswith('{}_'.format(self.project.name))
        ]

        # Two networks were created: back and front
        assert sorted(n['Name'].split('/')[-1] for n in networks) == [back_name, front_name]

        # lookup by ID instead of name in case of duplicates
        back_network = self.client.inspect_network(
            [n for n in networks if n['Name'] == back_name][0]['Id']
        )
        front_network = self.client.inspect_network(
            [n for n in networks if n['Name'] == front_name][0]['Id']
        )

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

    def test_up_missing_network(self):
        self.base_dir = 'tests/fixtures/networks'

        result = self.dispatch(
            ['-f', 'missing-network.yml', 'up', '-d'],
            returncode=1)

        assert 'Service "web" uses an undefined network "foo"' in result.stderr

    @no_cluster('container networks not supported in Swarm')
    def test_up_with_network_mode(self):
        c = self.client.create_container(
            'busybox', 'top', name='composetest_network_mode_container',
            host_config={}
        )
        self.addCleanup(self.client.remove_container, c, force=True)
        self.client.start(c)
        container_mode_source = 'container:{}'.format(c['Id'])

        filename = 'network-mode.yml'

        self.base_dir = 'tests/fixtures/networks'
        self._project = get_project(self.base_dir, [filename])

        self.dispatch(['-f', filename, 'up', '-d'], None)

        networks = [
            n for n in self.client.networks()
            if n['Name'].split('/')[-1].startswith('{}_'.format(self.project.name))
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
            self.client.create_network(name, attachable=True)

        self.dispatch(['-f', filename, 'up', '-d'])
        container = self.project.containers()[0]
        assert sorted(list(container.get('NetworkSettings.Networks'))) == sorted(network_names)

    def test_up_with_external_default_network(self):
        filename = 'external-default.yml'

        self.base_dir = 'tests/fixtures/networks'
        self._project = get_project(self.base_dir, [filename])

        result = self.dispatch(['-f', filename, 'up', '-d'], returncode=1)
        assert 'declared as external, but could not be found' in result.stderr

        networks = [
            n['Name'] for n in self.client.networks()
            if n['Name'].split('/')[-1].startswith('{}_'.format(self.project.name))
        ]
        assert not networks

        network_name = 'composetest_external_network'
        self.client.create_network(network_name, attachable=True)

        self.dispatch(['-f', filename, 'up', '-d'])
        container = self.project.containers()[0]
        assert list(container.get('NetworkSettings.Networks')) == [network_name]

    def test_up_with_network_labels(self):
        filename = 'network-label.yml'

        self.base_dir = 'tests/fixtures/networks'
        self._project = get_project(self.base_dir, [filename])

        self.dispatch(['-f', filename, 'up', '-d'], returncode=0)

        network_with_label = '{}_network_with_label'.format(self.project.name)

        networks = [
            n for n in self.client.networks()
            if n['Name'].split('/')[-1].startswith('{}_'.format(self.project.name))
        ]

        assert [n['Name'].split('/')[-1] for n in networks] == [network_with_label]
        assert 'label_key' in networks[0]['Labels']
        assert networks[0]['Labels']['label_key'] == 'label_val'

    def test_up_with_volume_labels(self):
        filename = 'volume-label.yml'

        self.base_dir = 'tests/fixtures/volumes'
        self._project = get_project(self.base_dir, [filename])

        self.dispatch(['-f', filename, 'up', '-d'], returncode=0)

        volume_with_label = '{}_volume_with_label'.format(self.project.name)

        volumes = [
            v for v in self.client.volumes().get('Volumes', [])
            if v['Name'].split('/')[-1].startswith('{}_'.format(self.project.name))
        ]

        assert {v['Name'].split('/')[-1] for v in volumes} == {volume_with_label}
        assert 'label_key' in volumes[0]['Labels']
        assert volumes[0]['Labels']['label_key'] == 'label_val'

    def test_up_no_services(self):
        self.base_dir = 'tests/fixtures/no-services'
        self.dispatch(['up', '-d'], None)

        network_names = [
            n['Name'] for n in self.client.networks()
            if n['Name'].split('/')[-1].startswith('{}_'.format(self.project.name))
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
        assert len(web.containers()) == 1
        assert len(db.containers()) == 1
        assert len(console.containers()) == 0

        # web has links
        web_container = web.containers()[0]
        assert web_container.get('HostConfig.Links')

    def test_up_with_net_is_invalid(self):
        self.base_dir = 'tests/fixtures/net-container'

        result = self.dispatch(
            ['-f', 'v2-invalid.yml', 'up', '-d'],
            returncode=1)

        assert "Unsupported config option for services.bar: 'net'" in result.stderr

    @no_cluster("Legacy networking not supported on Swarm")
    def test_up_with_net_v1(self):
        self.base_dir = 'tests/fixtures/net-container'
        self.dispatch(['up', '-d'], None)

        bar = self.project.get_service('bar')
        bar_container = bar.containers()[0]

        foo = self.project.get_service('foo')
        foo_container = foo.containers()[0]

        assert foo_container.get('HostConfig.NetworkMode') == 'container:{}'.format(
            bar_container.id
        )

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
        assert len(web.containers()) == 1
        assert len(db.containers()) == 0
        assert len(console.containers()) == 0

    def test_up_with_attach_dependencies(self):
        self.base_dir = 'tests/fixtures/echo-services-dependencies'
        result = self.dispatch(['up', '--attach-dependencies', '--no-color', 'simple'], None)
        simple_name = self.project.get_service('simple').containers(stopped=True)[0].name_without_project
        another_name = self.project.get_service('another').containers(
            stopped=True
        )[0].name_without_project

        assert '{}   | simple'.format(simple_name) in result.stdout
        assert '{}  | another'.format(another_name) in result.stdout

    def test_up_handles_aborted_dependencies(self):
        self.base_dir = 'tests/fixtures/abort-on-container-exit-dependencies'
        proc = start_process(
            self.base_dir,
            ['up', 'simple', '--attach-dependencies', '--abort-on-container-exit'])
        wait_on_condition(ContainerCountCondition(self.project, 0))
        proc.wait()
        assert proc.returncode == 1

    def test_up_with_force_recreate(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        assert len(service.containers()) == 1

        old_ids = [c.id for c in service.containers()]

        self.dispatch(['up', '-d', '--force-recreate'], None)
        assert len(service.containers()) == 1

        new_ids = [c.id for c in service.containers()]

        assert old_ids != new_ids

    def test_up_with_no_recreate(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        assert len(service.containers()) == 1

        old_ids = [c.id for c in service.containers()]

        self.dispatch(['up', '-d', '--no-recreate'], None)
        assert len(service.containers()) == 1

        new_ids = [c.id for c in service.containers()]

        assert old_ids == new_ids

    def test_up_with_force_recreate_and_no_recreate(self):
        self.dispatch(
            ['up', '-d', '--force-recreate', '--no-recreate'],
            returncode=1)

    def test_up_with_timeout(self):
        self.dispatch(['up', '-d', '-t', '1'])
        service = self.project.get_service('simple')
        another = self.project.get_service('another')
        assert len(service.containers()) == 1
        assert len(another.containers()) == 1

    @mock.patch.dict(os.environ)
    def test_up_with_ignore_remove_orphans(self):
        os.environ["COMPOSE_IGNORE_ORPHANS"] = "True"
        result = self.dispatch(['up', '-d', '--remove-orphans'], returncode=1)
        assert "COMPOSE_IGNORE_ORPHANS and --remove-orphans cannot be combined." in result.stderr

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
        assert proc.returncode == 0

    def test_up_handles_abort_on_container_exit_code(self):
        self.base_dir = 'tests/fixtures/abort-on-container-exit-1'
        proc = start_process(self.base_dir, ['up', '--abort-on-container-exit'])
        wait_on_condition(ContainerCountCondition(self.project, 0))
        proc.wait()
        assert proc.returncode == 1

    @no_cluster('Container PID mode does not work across clusters')
    def test_up_with_pid_mode(self):
        c = self.client.create_container(
            'busybox', 'top', name='composetest_pid_mode_container',
            host_config={}
        )
        self.addCleanup(self.client.remove_container, c, force=True)
        self.client.start(c)
        container_mode_source = 'container:{}'.format(c['Id'])

        self.base_dir = 'tests/fixtures/pid-mode'

        self.dispatch(['up', '-d'], None)

        service_mode_source = 'container:{}'.format(
            self.project.get_service('container').containers()[0].id)
        service_mode_container = self.project.get_service('service').containers()[0]
        assert service_mode_container.get('HostConfig.PidMode') == service_mode_source

        container_mode_container = self.project.get_service('container').containers()[0]
        assert container_mode_container.get('HostConfig.PidMode') == container_mode_source

        host_mode_container = self.project.get_service('host').containers()[0]
        assert host_mode_container.get('HostConfig.PidMode') == 'host'

    @no_cluster('Container IPC mode does not work across clusters')
    def test_up_with_ipc_mode(self):
        c = self.client.create_container(
            'busybox', 'top', name='composetest_ipc_mode_container',
            host_config={}
        )
        self.addCleanup(self.client.remove_container, c, force=True)
        self.client.start(c)
        container_mode_source = 'container:{}'.format(c['Id'])

        self.base_dir = 'tests/fixtures/ipc-mode'

        self.dispatch(['up', '-d'], None)

        service_mode_source = 'container:{}'.format(
            self.project.get_service('shareable').containers()[0].id)
        service_mode_container = self.project.get_service('service').containers()[0]
        assert service_mode_container.get('HostConfig.IpcMode') == service_mode_source

        container_mode_container = self.project.get_service('container').containers()[0]
        assert container_mode_container.get('HostConfig.IpcMode') == container_mode_source

        shareable_mode_container = self.project.get_service('shareable').containers()[0]
        assert shareable_mode_container.get('HostConfig.IpcMode') == 'shareable'

    def test_profiles_up_with_no_profile(self):
        self.base_dir = 'tests/fixtures/profiles'
        self.dispatch(['up'])

        containers = self.project.containers(stopped=True)
        service_names = [c.service for c in containers]

        assert 'foo' in service_names
        assert len(containers) == 1

    def test_profiles_up_with_profile(self):
        self.base_dir = 'tests/fixtures/profiles'
        self.dispatch(['--profile', 'test', 'up'])

        containers = self.project.containers(stopped=True)
        service_names = [c.service for c in containers]

        assert 'foo' in service_names
        assert 'bar' in service_names
        assert 'baz' in service_names
        assert len(containers) == 3

    def test_profiles_up_invalid_dependency(self):
        self.base_dir = 'tests/fixtures/profiles'
        result = self.dispatch(['--profile', 'debug', 'up'], returncode=1)

        assert ('Service "bar" was pulled in as a dependency of service "zot" '
                'but is not enabled by the active profiles.') in result.stderr

    def test_profiles_up_with_multiple_profiles(self):
        self.base_dir = 'tests/fixtures/profiles'
        self.dispatch(['--profile', 'debug', '--profile', 'test', 'up'])

        containers = self.project.containers(stopped=True)
        service_names = [c.service for c in containers]

        assert 'foo' in service_names
        assert 'bar' in service_names
        assert 'baz' in service_names
        assert 'zot' in service_names
        assert len(containers) == 4

    def test_profiles_up_with_profile_enabled_by_service(self):
        self.base_dir = 'tests/fixtures/profiles'
        self.dispatch(['up', 'bar'])

        containers = self.project.containers(stopped=True)
        service_names = [c.service for c in containers]

        assert 'bar' in service_names
        assert len(containers) == 1

    def test_profiles_up_with_dependency_and_profile_enabled_by_service(self):
        self.base_dir = 'tests/fixtures/profiles'
        self.dispatch(['up', 'baz'])

        containers = self.project.containers(stopped=True)
        service_names = [c.service for c in containers]

        assert 'bar' in service_names
        assert 'baz' in service_names
        assert len(containers) == 2

    def test_profiles_up_with_invalid_dependency_for_target_service(self):
        self.base_dir = 'tests/fixtures/profiles'
        result = self.dispatch(['up', 'zot'], returncode=1)

        assert ('Service "bar" was pulled in as a dependency of service "zot" '
                'but is not enabled by the active profiles.') in result.stderr

    def test_profiles_up_with_profile_for_dependency(self):
        self.base_dir = 'tests/fixtures/profiles'
        self.dispatch(['--profile', 'test', 'up', 'zot'])

        containers = self.project.containers(stopped=True)
        service_names = [c.service for c in containers]

        assert 'bar' in service_names
        assert 'zot' in service_names
        assert len(containers) == 2

    def test_profiles_up_with_merged_profiles(self):
        self.base_dir = 'tests/fixtures/profiles'
        self.dispatch(['-f', 'docker-compose.yml', '-f', 'merge-profiles.yml', 'up', 'zot'])

        containers = self.project.containers(stopped=True)
        service_names = [c.service for c in containers]

        assert 'bar' in service_names
        assert 'zot' in service_names
        assert len(containers) == 2

    def test_exec_without_tty(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d', 'console'])
        assert len(self.project.containers()) == 1

        stdout, stderr = self.dispatch(['exec', '-T', 'console', 'ls', '-1d', '/'])
        assert stderr == ""
        assert stdout == "/\n"

    @mock.patch.dict(os.environ)
    def test_exec_novalue_var_dotenv_file(self):
        os.environ['MYVAR'] = 'SUCCESS'
        self.base_dir = 'tests/fixtures/exec-novalue-var'
        self.dispatch(['up', '-d'])
        assert len(self.project.containers()) == 1

        stdout, stderr = self.dispatch(['exec', '-T', 'nginx', 'env'])
        assert 'CHECK_VAR=SUCCESS' in stdout
        assert not stderr

    def test_exec_detach_long_form(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '--detach', 'console'])
        assert len(self.project.containers()) == 1

        stdout, stderr = self.dispatch(['exec', '-T', 'console', 'ls', '-1d', '/'])
        assert stderr == ""
        assert stdout == "/\n"

    def test_exec_custom_user(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d', 'console'])
        assert len(self.project.containers()) == 1

        stdout, stderr = self.dispatch(['exec', '-T', '--user=operator', 'console', 'whoami'])
        assert stdout == "operator\n"
        assert stderr == ""

    def test_exec_workdir(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        os.environ['COMPOSE_API_VERSION'] = '1.35'
        self.dispatch(['up', '-d', 'console'])
        assert len(self.project.containers()) == 1

        stdout, stderr = self.dispatch(['exec', '-T', '--workdir', '/etc', 'console', 'ls'])
        assert 'passwd' in stdout

    def test_exec_service_with_environment_overridden(self):
        name = 'service'
        self.base_dir = 'tests/fixtures/environment-exec'
        self.dispatch(['up', '-d'])
        assert len(self.project.containers()) == 1

        stdout, stderr = self.dispatch([
            'exec',
            '-T',
            '-e', 'foo=notbar',
            '--env', 'alpha=beta',
            name,
            'env',
        ])

        # env overridden
        assert 'foo=notbar' in stdout
        # keep environment from yaml
        assert 'hello=world' in stdout
        # added option from command line
        assert 'alpha=beta' in stdout

        assert stderr == ''

    def test_run_service_without_links(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['run', 'console', '/bin/true'])
        assert len(self.project.containers()) == 0

        # Ensure stdin/out was open
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        config = container.inspect()['Config']
        assert config['AttachStderr']
        assert config['AttachStdout']
        assert config['AttachStdin']

    def test_run_service_with_links(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['run', 'web', '/bin/true'], None)
        db = self.project.get_service('db')
        console = self.project.get_service('console')
        assert len(db.containers()) == 1
        assert len(console.containers()) == 0

    def test_run_service_with_dependencies(self):
        self.base_dir = 'tests/fixtures/v2-dependencies'
        self.dispatch(['run', 'web', '/bin/true'], None)
        db = self.project.get_service('db')
        console = self.project.get_service('console')
        assert len(db.containers()) == 1
        assert len(console.containers()) == 0

    def test_run_service_with_unhealthy_dependencies(self):
        self.base_dir = 'tests/fixtures/v2-unhealthy-dependencies'
        result = self.dispatch(['run', 'web', '/bin/true'], returncode=1)
        assert re.search(
            re.compile('for web .*is unhealthy.*', re.MULTILINE),
            result.stderr
        )

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
        assert len(db.containers()) == 0

    def test_run_does_not_recreate_linked_containers(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d', 'db'])
        db = self.project.get_service('db')
        assert len(db.containers()) == 1

        old_ids = [c.id for c in db.containers()]

        self.dispatch(['run', 'web', '/bin/true'], None)
        assert len(db.containers()) == 1

        new_ids = [c.id for c in db.containers()]

        assert old_ids == new_ids

    def test_run_without_command(self):
        self.base_dir = 'tests/fixtures/commands-composefile'
        self.check_build('tests/fixtures/simple-dockerfile', tag='composetest_test')

        self.dispatch(['run', 'implicit'])
        service = self.project.get_service('implicit')
        containers = service.containers(stopped=True, one_off=OneOffFilter.only)
        assert [c.human_readable_command for c in containers] == ['/bin/sh -c echo "success"']

        self.dispatch(['run', 'explicit'])
        service = self.project.get_service('explicit')
        containers = service.containers(stopped=True, one_off=OneOffFilter.only)
        assert [c.human_readable_command for c in containers] == ['/bin/true']

    @pytest.mark.skipif(SWARM_SKIP_RM_VOLUMES, reason='Swarm DELETE /containers/<id> bug')
    def test_run_rm(self):
        self.base_dir = 'tests/fixtures/volume'
        proc = start_process(self.base_dir, ['run', '--rm', 'test'])
        service = self.project.get_service('test')
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'volume_test_run_*',
            'running')
        )
        containers = service.containers(one_off=OneOffFilter.only)
        assert len(containers) == 1
        mounts = containers[0].get('Mounts')
        for mount in mounts:
            if mount['Destination'] == '/container-path':
                anonymous_name = mount['Name']
                break
        os.kill(proc.pid, signal.SIGINT)
        wait_on_process(proc, 1)

        assert len(service.containers(stopped=True, one_off=OneOffFilter.only)) == 0

        volumes = self.client.volumes()['Volumes']
        assert volumes is not None
        for volume in service.options.get('volumes'):
            if volume.internal == '/container-named-path':
                name = volume.external
                break
        volume_names = [v['Name'].split('/')[-1] for v in volumes]
        assert name in volume_names
        assert anonymous_name not in volume_names

    def test_run_service_with_dockerfile_entrypoint(self):
        self.base_dir = 'tests/fixtures/entrypoint-dockerfile'
        self.dispatch(['run', 'test'])
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        assert container.get('Config.Entrypoint') == ['printf']
        assert container.get('Config.Cmd') == ['default', 'args']

    def test_run_service_with_unset_entrypoint(self):
        self.base_dir = 'tests/fixtures/entrypoint-dockerfile'
        self.dispatch(['run', '--entrypoint=""', 'test', 'true'])
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        assert container.get('Config.Entrypoint') is None
        assert container.get('Config.Cmd') == ['true']

        self.dispatch(['run', '--entrypoint', '""', 'test', 'true'])
        container = self.project.containers(stopped=True, one_off=OneOffFilter.only)[0]
        assert container.get('Config.Entrypoint') is None
        assert container.get('Config.Cmd') == ['true']

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
        assert user == container.get('Config.User')

    def test_run_service_with_user_overridden_short_form(self):
        self.base_dir = 'tests/fixtures/user-composefile'
        name = 'service'
        user = 'sshd'
        self.dispatch(['run', '-u', user, name], returncode=1)
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=OneOffFilter.only)[0]
        assert user == container.get('Config.User')

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
        assert 'notbar' == container.environment['foo']
        # keep environment from yaml
        assert 'world' == container.environment['hello']
        # added option from command line
        assert 'beta' == container.environment['alpha']
        # make sure a value with a = don't crash out
        assert 'moto=bobo' == container.environment['allo']

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
        assert port_random is None
        assert port_assigned is None

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
        assert port_random is not None
        assert port_assigned.endswith(':49152')
        assert port_range[0].endswith(':49153')
        assert port_range[1].endswith(':49154')

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
        assert port_short.endswith(':30000')
        assert port_full.endswith(':30001')

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
        assert port_short == "127.0.0.1:30000"
        assert port_full == "127.0.0.1:30001"

    def test_run_with_expose_ports(self):
        # create one off container
        self.base_dir = 'tests/fixtures/expose-composefile'
        self.dispatch(['run', '-d', '--service-ports', 'simple'])
        container = self.project.get_service('simple').containers(one_off=OneOffFilter.only)[0]

        ports = container.ports
        assert len(ports) == 9
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
        assert container.name == name

    def test_run_service_with_workdir_overridden(self):
        self.base_dir = 'tests/fixtures/run-workdir'
        name = 'service'
        workdir = '/var'
        self.dispatch(['run', '--workdir={workdir}'.format(workdir=workdir), name])
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=True)[0]
        assert workdir == container.get('Config.WorkingDir')

    def test_run_service_with_workdir_overridden_short_form(self):
        self.base_dir = 'tests/fixtures/run-workdir'
        name = 'service'
        workdir = '/var'
        self.dispatch(['run', '-w', workdir, name])
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=True)[0]
        assert workdir == container.get('Config.WorkingDir')

    def test_run_service_with_use_aliases(self):
        filename = 'network-aliases.yml'
        self.base_dir = 'tests/fixtures/networks'
        self.dispatch(['-f', filename, 'run', '-d', '--use-aliases', 'web', 'top'])

        back_name = '{}_back'.format(self.project.name)
        front_name = '{}_front'.format(self.project.name)

        web_container = self.project.get_service('web').containers(one_off=OneOffFilter.only)[0]

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
                aliases = set(config['Aliases'] or []) - {container.short_id}
                assert not aliases

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
            aliases = set(config['Aliases'] or []) - {container.short_id}
            assert not aliases

        assert self.lookup(container, 'app')
        assert self.lookup(container, 'db')

    def test_run_handles_sigint(self):
        proc = start_process(self.base_dir, ['run', '-T', 'simple', 'top'])
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simple-composefile_simple_run_*',
            'running'))

        os.kill(proc.pid, signal.SIGINT)
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simple-composefile_simple_run_*',
            'exited'))

    def test_run_handles_sigterm(self):
        proc = start_process(self.base_dir, ['run', '-T', 'simple', 'top'])
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simple-composefile_simple_run_*',
            'running'))

        os.kill(proc.pid, signal.SIGTERM)
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simple-composefile_simple_run_*',
            'exited'))

    def test_run_handles_sighup(self):
        proc = start_process(self.base_dir, ['run', '-T', 'simple', 'top'])
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simple-composefile_simple_run_*',
            'running'))

        os.kill(proc.pid, signal.SIGHUP)
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simple-composefile_simple_run_*',
            'exited'))

    @mock.patch.dict(os.environ)
    def test_run_unicode_env_values_from_system(self):
        value = 'ą, ć, ę, ł, ń, ó, ś, ź, ż'
        os.environ['BAR'] = value
        self.base_dir = 'tests/fixtures/unicode-environment'
        self.dispatch(['run', 'simple'])

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

    def test_run_label_flag(self):
        self.base_dir = 'tests/fixtures/run-labels'
        name = 'service'
        self.dispatch(['run', '-l', 'default', '--label', 'foo=baz', name, '/bin/true'])
        service = self.project.get_service(name)
        container, = service.containers(stopped=True, one_off=OneOffFilter.only)
        labels = container.labels
        assert labels['default'] == ''
        assert labels['foo'] == 'baz'
        assert labels['hello'] == 'world'

    def test_rm(self):
        service = self.project.get_service('simple')
        service.create_container()
        kill_service(service)
        assert len(service.containers(stopped=True)) == 1
        self.dispatch(['rm', '--force'], None)
        assert len(service.containers(stopped=True)) == 0
        service = self.project.get_service('simple')
        service.create_container()
        kill_service(service)
        assert len(service.containers(stopped=True)) == 1
        self.dispatch(['rm', '-f'], None)
        assert len(service.containers(stopped=True)) == 0
        service = self.project.get_service('simple')
        service.create_container()
        self.dispatch(['rm', '-fs'], None)
        assert len(service.containers(stopped=True)) == 0

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
        assert len(service.containers(stopped=True)) == 1
        assert len(service.containers(stopped=True, one_off=OneOffFilter.only)) == 1
        self.dispatch(['rm', '-f'], None)
        assert len(service.containers(stopped=True)) == 0
        assert len(service.containers(stopped=True, one_off=OneOffFilter.only)) == 0

        service.create_container(one_off=False)
        service.create_container(one_off=True)
        kill_service(service)
        assert len(service.containers(stopped=True)) == 1
        assert len(service.containers(stopped=True, one_off=OneOffFilter.only)) == 1
        self.dispatch(['rm', '-f', '--all'], None)
        assert len(service.containers(stopped=True)) == 0
        assert len(service.containers(stopped=True, one_off=OneOffFilter.only)) == 0

    def test_stop(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        assert len(service.containers()) == 1
        assert service.containers()[0].is_running

        self.dispatch(['stop', '-t', '1'], None)

        assert len(service.containers(stopped=True)) == 1
        assert not service.containers(stopped=True)[0].is_running

    def test_stop_signal(self):
        self.base_dir = 'tests/fixtures/stop-signal-composefile'
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        assert len(service.containers()) == 1
        assert service.containers()[0].is_running

        self.dispatch(['stop', '-t', '1'], None)
        assert len(service.containers(stopped=True)) == 1
        assert not service.containers(stopped=True)[0].is_running
        assert service.containers(stopped=True)[0].exit_code == 0

    def test_start_no_containers(self):
        result = self.dispatch(['start'], returncode=1)
        assert 'failed' in result.stderr
        assert 'No containers to start' in result.stderr

    def test_up_logging(self):
        self.base_dir = 'tests/fixtures/logging-composefile'
        self.dispatch(['up', '-d'])
        simple = self.project.get_service('simple').containers()[0]
        log_config = simple.get('HostConfig.LogConfig')
        assert log_config
        assert log_config.get('Type') == 'none'

        another = self.project.get_service('another').containers()[0]
        log_config = another.get('HostConfig.LogConfig')
        assert log_config
        assert log_config.get('Type') == 'json-file'
        assert log_config.get('Config')['max-size'] == '10m'

    def test_up_logging_legacy(self):
        self.base_dir = 'tests/fixtures/logging-composefile-legacy'
        self.dispatch(['up', '-d'])
        simple = self.project.get_service('simple').containers()[0]
        log_config = simple.get('HostConfig.LogConfig')
        assert log_config
        assert log_config.get('Type') == 'none'

        another = self.project.get_service('another').containers()[0]
        log_config = another.get('HostConfig.LogConfig')
        assert log_config
        assert log_config.get('Type') == 'json-file'
        assert log_config.get('Config')['max-size'] == '10m'

    def test_pause_unpause(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        assert not service.containers()[0].is_paused

        self.dispatch(['pause'], None)
        assert service.containers()[0].is_paused

        self.dispatch(['unpause'], None)
        assert not service.containers()[0].is_paused

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

        if not is_cluster(self.client):
            assert result.stdout.count('\n') == 5
        else:
            # Sometimes logs are picked up from old containers that haven't yet
            # been removed (removal in Swarm is async)
            assert result.stdout.count('\n') >= 5

        assert 'simple' in result.stdout
        assert 'another' in result.stdout
        assert 'exited with code 0' in result.stdout

    @pytest.mark.skip(reason="race condition between up and logs")
    def test_logs_follow_logs_from_new_containers(self):
        self.base_dir = 'tests/fixtures/logs-composefile'
        self.dispatch(['up', '-d', 'simple'])

        proc = start_process(self.base_dir, ['logs', '-f'])

        self.dispatch(['up', '-d', 'another'])
        another_name = self.project.get_service('another').get_container().name_without_project
        wait_on_condition(
            ContainerStateCondition(
                self.project.client,
                'logs-composefile_another_*',
                'exited'
            )
        )

        simple_name = self.project.get_service('simple').get_container().name_without_project
        self.dispatch(['kill', 'simple'])

        result = wait_on_process(proc)

        assert 'hello' in result.stdout
        assert 'test' in result.stdout
        assert '{} exited with code 0'.format(another_name) in result.stdout
        assert '{} exited with code 137'.format(simple_name) in result.stdout

    @pytest.mark.skip(reason="race condition between up and logs")
    def test_logs_follow_logs_from_restarted_containers(self):
        self.base_dir = 'tests/fixtures/logs-restart-composefile'
        proc = start_process(self.base_dir, ['up'])

        wait_on_condition(
            ContainerStateCondition(
                self.project.client,
                'logs-restart-composefile_another_*',
                'exited'
            )
        )
        self.dispatch(['kill', 'simple'])

        result = wait_on_process(proc)

        assert result.stdout.count(
            r'logs-restart-composefile_another_1 exited with code 1'
        ) == 3
        assert result.stdout.count('world') == 3

    @pytest.mark.skip(reason="race condition between up and logs")
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
        assert re.search(r'(\d{4})-(\d{2})-(\d{2})T(\d{2})\:(\d{2})\:(\d{2})', result.stdout)

    def test_logs_tail(self):
        self.base_dir = 'tests/fixtures/logs-tail-composefile'
        self.dispatch(['up'])

        result = self.dispatch(['logs', '--tail', '2'])
        assert 'y\n' in result.stdout
        assert 'z\n' in result.stdout
        assert 'w\n' not in result.stdout
        assert 'x\n' not in result.stdout

    def test_kill(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        assert len(service.containers()) == 1
        assert service.containers()[0].is_running

        self.dispatch(['kill'], None)

        assert len(service.containers(stopped=True)) == 1
        assert not service.containers(stopped=True)[0].is_running

    def test_kill_signal_sigstop(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        assert len(service.containers()) == 1
        assert service.containers()[0].is_running

        self.dispatch(['kill', '-s', 'SIGSTOP'], None)

        assert len(service.containers()) == 1
        # The container is still running. It has only been paused
        assert service.containers()[0].is_running

    def test_kill_stopped_service(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.dispatch(['kill', '-s', 'SIGSTOP'], None)
        assert service.containers()[0].is_running

        self.dispatch(['kill', '-s', 'SIGKILL'], None)

        assert len(service.containers(stopped=True)) == 1
        assert not service.containers(stopped=True)[0].is_running

    def test_restart(self):
        service = self.project.get_service('simple')
        container = service.create_container()
        service.start_container(container)
        started_at = container.dictionary['State']['StartedAt']
        self.dispatch(['restart', '-t', '1'], None)
        container.inspect()
        assert container.dictionary['State']['FinishedAt'] != '0001-01-01T00:00:00Z'
        assert container.dictionary['State']['StartedAt'] != started_at

    def test_restart_stopped_container(self):
        service = self.project.get_service('simple')
        container = service.create_container()
        container.start()
        container.kill()
        assert len(service.containers(stopped=True)) == 1
        self.dispatch(['restart', '-t', '1'], None)
        assert len(service.containers(stopped=False)) == 1

    def test_restart_no_containers(self):
        result = self.dispatch(['restart'], returncode=1)
        assert 'No containers to restart' in result.stderr

    def test_scale(self):
        project = self.project

        self.dispatch(['scale', 'simple=1'])
        assert len(project.get_service('simple').containers()) == 1

        self.dispatch(['scale', 'simple=3', 'another=2'])
        assert len(project.get_service('simple').containers()) == 3
        assert len(project.get_service('another').containers()) == 2

        self.dispatch(['scale', 'simple=1', 'another=1'])
        assert len(project.get_service('simple').containers()) == 1
        assert len(project.get_service('another').containers()) == 1

        self.dispatch(['scale', 'simple=1', 'another=1'])
        assert len(project.get_service('simple').containers()) == 1
        assert len(project.get_service('another').containers()) == 1

        self.dispatch(['scale', 'simple=0', 'another=0'])
        assert len(project.get_service('simple').containers()) == 0
        assert len(project.get_service('another').containers()) == 0

    def test_up_scale_scale_up(self):
        self.base_dir = 'tests/fixtures/scale'
        project = self.project

        self.dispatch(['up', '-d'])
        assert len(project.get_service('web').containers()) == 2
        assert len(project.get_service('db').containers()) == 1
        assert len(project.get_service('worker').containers()) == 0

        self.dispatch(['up', '-d', '--scale', 'web=3', '--scale', 'worker=1'])
        assert len(project.get_service('web').containers()) == 3
        assert len(project.get_service('db').containers()) == 1
        assert len(project.get_service('worker').containers()) == 1

    def test_up_scale_scale_down(self):
        self.base_dir = 'tests/fixtures/scale'
        project = self.project

        self.dispatch(['up', '-d'])
        assert len(project.get_service('web').containers()) == 2
        assert len(project.get_service('db').containers()) == 1
        assert len(project.get_service('worker').containers()) == 0

        self.dispatch(['up', '-d', '--scale', 'web=1'])
        assert len(project.get_service('web').containers()) == 1
        assert len(project.get_service('db').containers()) == 1
        assert len(project.get_service('worker').containers()) == 0

    def test_up_scale_reset(self):
        self.base_dir = 'tests/fixtures/scale'
        project = self.project

        self.dispatch(['up', '-d', '--scale', 'web=3', '--scale', 'db=3', '--scale', 'worker=3'])
        assert len(project.get_service('web').containers()) == 3
        assert len(project.get_service('db').containers()) == 3
        assert len(project.get_service('worker').containers()) == 3

        self.dispatch(['up', '-d'])
        assert len(project.get_service('web').containers()) == 2
        assert len(project.get_service('db').containers()) == 1
        assert len(project.get_service('worker').containers()) == 0

    def test_up_scale_to_zero(self):
        self.base_dir = 'tests/fixtures/scale'
        project = self.project

        self.dispatch(['up', '-d'])
        assert len(project.get_service('web').containers()) == 2
        assert len(project.get_service('db').containers()) == 1
        assert len(project.get_service('worker').containers()) == 0

        self.dispatch(['up', '-d', '--scale', 'web=0', '--scale', 'db=0', '--scale', 'worker=0'])
        assert len(project.get_service('web').containers()) == 0
        assert len(project.get_service('db').containers()) == 0
        assert len(project.get_service('worker').containers()) == 0

    def test_port(self):
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['up', '-d'], None)
        container = self.project.get_service('simple').get_container()

        def get_port(number):
            result = self.dispatch(['port', 'simple', str(number)])
            return result.stdout.rstrip()

        assert get_port(3000) == container.get_local_port(3000)
        assert ':49152' in get_port(3001)
        assert ':49153' in get_port(3002)

    def test_expanded_port(self):
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['-f', 'expanded-notation.yml', 'up', '-d'])
        container = self.project.get_service('simple').get_container()

        def get_port(number):
            result = self.dispatch(['port', 'simple', str(number)])
            return result.stdout.rstrip()

        assert get_port(3000) == container.get_local_port(3000)
        assert ':53222' in get_port(3001)
        assert ':53223' in get_port(3002)

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

        assert get_port(3000) in (containers[0].get_local_port(3000), containers[1].get_local_port(3000))
        assert get_port(3000, index=containers[0].number) == containers[0].get_local_port(3000)
        assert get_port(3000, index=containers[1].number) == containers[1].get_local_port(3000)
        assert get_port(3002) == ""

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
                    '{} {}'.format(str_iso_date, str_iso_time),
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
        expected_template = ' container {} {}'
        expected_meta_info = ['image=busybox:1.27.2', 'name=simple-composefile_simple_']

        assert expected_template.format('create', container.id) in lines[0]
        assert expected_template.format('start', container.id) in lines[1]
        for line in lines:
            for info in expected_meta_info:
                assert info in line

        assert has_timestamp(lines[0])

    def test_env_file_relative_to_compose_file(self):
        config_path = os.path.abspath('tests/fixtures/env-file/docker-compose.yml')
        self.dispatch(['-f', config_path, 'up', '-d'], None)
        self._project = get_project(self.base_dir, [config_path])

        containers = self.project.containers(stopped=True)
        assert len(containers) == 1
        assert "FOO=1" in containers[0].get('Config.Env')

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
        assert len(containers) == 2

        web, db = containers
        assert web.human_readable_command == 'top'
        assert db.human_readable_command == 'top'

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
        assert len(containers) == 3

        web, other, db = containers
        assert web.human_readable_command == 'top'
        assert db.human_readable_command == 'top'
        assert other.human_readable_command == 'top'

    def test_up_with_extends(self):
        self.base_dir = 'tests/fixtures/extends'
        self.dispatch(['up', '-d'], None)

        assert {s.name for s in self.project.services} == {'mydb', 'myweb'}

        # Sort by name so we get [db, web]
        containers = sorted(
            self.project.containers(stopped=True),
            key=lambda c: c.name,
        )

        assert len(containers) == 2
        web = containers[1]
        db_name = containers[0].name_without_project

        assert set(get_links(web)) == {'db', db_name, 'extends_{}'.format(db_name)}

        expected_env = {"FOO=1", "BAR=2", "BAZ=2"}
        assert expected_env <= set(web.get('Config.Env'))

    def test_top_services_not_running(self):
        self.base_dir = 'tests/fixtures/top'
        result = self.dispatch(['top'])
        assert len(result.stdout) == 0

    def test_top_services_running(self):
        self.base_dir = 'tests/fixtures/top'
        self.dispatch(['up', '-d'])
        result = self.dispatch(['top'])

        assert 'top_service_a' in result.stdout
        assert 'top_service_b' in result.stdout
        assert 'top_not_a_service' not in result.stdout

    def test_top_processes_running(self):
        self.base_dir = 'tests/fixtures/top'
        self.dispatch(['up', '-d'])
        result = self.dispatch(['top'])
        assert result.stdout.count("top") == 4

    def test_forward_exitval(self):
        self.base_dir = 'tests/fixtures/exit-code-from'
        proc = start_process(
            self.base_dir,
            ['up', '--abort-on-container-exit', '--exit-code-from', 'another']
        )

        result = wait_on_process(proc, returncode=1)
        assert 'exit-code-from_another_1 exited with code 1' in result.stdout

    def test_exit_code_from_signal_stop(self):
        self.base_dir = 'tests/fixtures/exit-code-from'
        proc = start_process(
            self.base_dir,
            ['up', '--abort-on-container-exit', '--exit-code-from', 'simple']
        )
        result = wait_on_process(proc, returncode=137)  # SIGKILL
        name = self.project.get_service('another').containers(stopped=True)[0].name_without_project
        assert '{} exited with code 1'.format(name) in result.stdout

    def test_images(self):
        self.project.get_service('simple').create_container()
        result = self.dispatch(['images'])
        assert 'busybox' in result.stdout
        assert 'simple-composefile_simple_' in result.stdout

    def test_images_default_composefile(self):
        self.base_dir = 'tests/fixtures/multiple-composefiles'
        self.dispatch(['up', '-d'])
        result = self.dispatch(['images'])

        assert 'busybox' in result.stdout
        assert '_another_1' in result.stdout
        assert '_simple_1' in result.stdout

    @mock.patch.dict(os.environ)
    def test_images_tagless_image(self):
        self.base_dir = 'tests/fixtures/tagless-image'
        stream = self.client.build(self.base_dir, decode=True)
        img_id = None
        for data in stream:
            if 'aux' in data:
                img_id = data['aux']['ID']
                break
            if 'stream' in data and 'Successfully built' in data['stream']:
                img_id = self.client.inspect_image(data['stream'].split(' ')[2].strip())['Id']

        assert img_id

        os.environ['IMAGE_ID'] = img_id
        self.project.get_service('foo').create_container()
        result = self.dispatch(['images'])
        assert '<none>' in result.stdout
        assert 'tagless-image_foo_1' in result.stdout

    def test_up_with_override_yaml(self):
        self.base_dir = 'tests/fixtures/override-yaml-files'
        self._project = get_project(self.base_dir, [])
        self.dispatch(['up', '-d'], None)

        containers = self.project.containers()
        assert len(containers) == 2

        web, db = containers
        assert web.human_readable_command == 'sleep 100'
        assert db.human_readable_command == 'top'

    def test_up_with_duplicate_override_yaml_files(self):
        self.base_dir = 'tests/fixtures/duplicate-override-yaml-files'
        with pytest.raises(DuplicateOverrideFileFound):
            get_project(self.base_dir, [])
        self.base_dir = None

    def test_images_use_service_tag(self):
        pull_busybox(self.client)
        self.base_dir = 'tests/fixtures/images-service-tag'
        self.dispatch(['up', '-d', '--build'])
        result = self.dispatch(['images'])

        assert re.search(r'foo1.+test[ \t]+dev', result.stdout) is not None
        assert re.search(r'foo2.+test[ \t]+prod', result.stdout) is not None
        assert re.search(r'foo3.+test[ \t]+latest', result.stdout) is not None

    def test_build_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        result = self.dispatch(['build', '--pull', '--', '--test-service'])

        assert BUILD_PULL_TEXT in result.stdout

    def test_events_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        events_proc = start_process(self.base_dir, ['events', '--json', '--', '--test-service'])
        self.dispatch(['up', '-d', '--', '--test-service'])
        wait_on_condition(ContainerCountCondition(self.project, 1))

        os.kill(events_proc.pid, signal.SIGINT)
        result = wait_on_process(events_proc, returncode=1)
        lines = [json.loads(line) for line in result.stdout.rstrip().split('\n')]
        assert Counter(e['action'] for e in lines) == {'create': 1, 'start': 1}

    def test_exec_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        self.dispatch(['up', '-d', '--', '--test-service'])
        assert len(self.project.containers()) == 1

        stdout, stderr = self.dispatch(['exec', '-T', '--', '--test-service', 'ls', '-1d', '/'])

        assert stderr == ""
        assert stdout == "/\n"

    def test_images_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        self.dispatch(['up', '-d', '--', '--test-service'])
        result = self.dispatch(['images', '--', '--test-service'])

        assert "busybox" in result.stdout

    def test_kill_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        self.dispatch(['up', '-d', '--', '--test-service'])
        service = self.project.get_service('--test-service')

        assert len(service.containers()) == 1
        assert service.containers()[0].is_running

        self.dispatch(['kill', '--', '--test-service'])

        assert len(service.containers(stopped=True)) == 1
        assert not service.containers(stopped=True)[0].is_running

    def test_logs_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        self.dispatch(['up', '-d', '--', '--log-service'])
        result = self.dispatch(['logs', '--', '--log-service'])

        assert 'hello' in result.stdout
        assert 'exited with' not in result.stdout

    def test_port_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        self.dispatch(['up', '-d', '--', '--test-service'])
        result = self.dispatch(['port', '--', '--test-service', '80'])

        assert result.stdout.strip() == "0.0.0.0:8080"

    def test_ps_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        self.dispatch(['up', '-d', '--', '--test-service'])

        result = self.dispatch(['ps', '--', '--test-service'])

        assert 'flag-as-service-name_--test-service_1' in result.stdout

    def test_pull_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        result = self.dispatch(['pull', '--', '--test-service'])

        assert 'Pulling --test-service' in result.stderr
        assert 'failed' not in result.stderr

    def test_rm_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        self.dispatch(['up', '--no-start', '--', '--test-service'])
        service = self.project.get_service('--test-service')
        assert len(service.containers(stopped=True)) == 1

        self.dispatch(['rm', '--force', '--', '--test-service'])
        assert len(service.containers(stopped=True)) == 0

    def test_run_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        result = self.dispatch(['run', '--no-deps', '--', '--test-service', 'echo', '-hello'])

        assert 'hello' in result.stdout
        assert len(self.project.containers()) == 0

    def test_stop_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        self.dispatch(['up', '-d', '--', '--test-service'])
        service = self.project.get_service('--test-service')
        assert len(service.containers()) == 1
        assert service.containers()[0].is_running

        self.dispatch(['stop', '-t', '1', '--', '--test-service'])

        assert len(service.containers(stopped=True)) == 1
        assert not service.containers(stopped=True)[0].is_running

    def test_restart_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        self.dispatch(['up', '-d', '--', '--test-service'])
        service = self.project.get_service('--test-service')
        assert len(service.containers()) == 1
        assert service.containers()[0].is_running

        self.dispatch(['restart', '-t', '1', '--', '--test-service'])

        assert len(service.containers()) == 1
        assert service.containers()[0].is_running

    def test_up_with_stop_process_flag(self):
        self.base_dir = 'tests/fixtures/flag-as-service-name'
        self.dispatch(['up', '-d', '--', '--test-service', '--log-service'])

        service = self.project.get_service('--test-service')
        another = self.project.get_service('--log-service')
        assert len(service.containers()) == 1
        assert len(another.containers()) == 1

    def test_up_no_log_prefix(self):
        self.base_dir = 'tests/fixtures/echo-services'
        result = self.dispatch(['up', '--no-log-prefix'])

        assert 'simple' in result.stdout
        assert 'another' in result.stdout
        assert 'exited with code 0' in result.stdout
        assert 'exited with code 0' in result.stdout
