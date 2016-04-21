from __future__ import absolute_import
from __future__ import unicode_literals

import os
import sys

import docker
import mock
import pytest

import compose
from compose.config.environment import Environment
from compose.config.func_map import InvalidHelperFunction
from compose.config.func_map import PosixOnlyHelperException
from compose.config.interpolation import interpolate_environment_variables
from compose.const import IS_WINDOWS_PLATFORM


@pytest.yield_fixture
def mock_env():
    with mock.patch.dict(os.environ):
        os.environ['USER'] = 'jenny'
        os.environ['FOO'] = 'bar'
        yield


def get_docker_version():
    return ".".join([str(i) for i in docker.version_info])


def get_uid():
    if not IS_WINDOWS_PLATFORM:
        return os.getuid()
    # donot raise exception and return
    # faked uid
    return 1234


def get_guid():
    if not IS_WINDOWS_PLATFORM:
        return os.getgid()
    # donot raise exception and return
    # faked gid
    return 1234


def test_interpolate_environment_variables_in_services(mock_env):
    services = {
        'servicea': {
            'image': 'example:${USER}',
            'volumes': ['$FOO:/target'],
            'logging': {
                'driver': '${FOO}',
                'options': {
                    'user': '$USER',
                }
            }
        }
    }
    expected = {
        'servicea': {
            'image': 'example:jenny',
            'volumes': ['bar:/target'],
            'logging': {
                'driver': 'bar',
                'options': {
                    'user': 'jenny',
                }
            }
        }
    }
    assert interpolate_environment_variables(
        services, 'service', Environment.from_env_file(None)
    ) == expected


def test_interpolate_environment_variables_in_volumes(mock_env):
    volumes = {
        'data': {
            'driver': '$FOO',
            'driver_opts': {
                'max': 2,
                'user': '${USER}'
            }
        },
        'other': None,
    }
    expected = {
        'data': {
            'driver': 'bar',
            'driver_opts': {
                'max': 2,
                'user': 'jenny'
            }
        },
        'other': {},
    }
    assert interpolate_environment_variables(
        volumes, 'volume', Environment.from_env_file(None)
    ) == expected


def test_invoke_funcmap_in_services(mock_env):

    services = {
        'servicea': {
            'image': '@{get_host_platform}:latest',
            'environment': {
                'COMPOSE_VERSION': '@{get_compose_version}',
                'DOCKER_VERSION': '@{get_docker_version}',
            }
        }
    }
    expected = {
        'servicea': {
            'image': '%s:latest' % sys.platform,
            'environment': {
                'COMPOSE_VERSION': compose.__version__,
                'DOCKER_VERSION': get_docker_version(),
            }
        }
    }

    assert interpolate_environment_variables(
        services, 'servicea', Environment.from_env_file(None)
    ) == expected


def test_invoke_posix_funcmap_in_services(mock_env):

    services = {
        'servicea': {
            'user': '@{get_user_id}:@{get_group_id}',
        }
    }
    expected = {
        'servicea': {
            'user': '%d:%d' % (get_uid(), get_guid()),
        }
    }

    if not IS_WINDOWS_PLATFORM:
        assert interpolate_environment_variables(
            services, 'servicea', Environment.from_env_file(None)
        ) == expected
    else:
        try:
            interpolate_environment_variables(
                services, 'servicea', Environment.from_env_file(None)
            )
        except PosixOnlyHelperException:
            pass


def test_inihbate_invoke_funcmap_in_services(mock_env):

    services = {
        'servicea': {
            'image': '@@{get_host_platform}:doubled'
        }
    }
    expected = {
        'servicea': {
            'image': '@{get_host_platform}:doubled'
        }
    }
    assert interpolate_environment_variables(
        services, 'servicea', Environment.from_env_file(None)
    ) == expected


def test_calling_unknown_helper_function(mock_env):

    services = {
        'servicea': {
            'image': '@{unknown_function_test}:latest',
        }
    }

    try:
        assert interpolate_environment_variables(
            services, 'servicea', Environment.from_env_file(None)
        )
        raise Exception("Should failed")
    except InvalidHelperFunction:
        pass
