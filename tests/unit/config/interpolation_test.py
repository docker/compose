from __future__ import absolute_import
from __future__ import unicode_literals

import os

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


@pytest.yield_fixture
def mock_func_map():
    def _uid():
        return 5555

    def _gid():
        return 5556

    def _docker_version():
        return "1.2.3"

    def _compose_version():
        return compose.__version__

    def _platform():
        return 'mocked_platform'

    with mock.patch.dict(compose.config.interpolation.func_map):
        compose.config.interpolation.func_map = {
            'get_user_id': _uid,
            'get_group_id': _gid,
            'get_compose_version': _compose_version,
            'get_docker_version': _docker_version,
            'get_host_platform': _platform
        }
        yield


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


def test_invoke_funcmap_in_services(mock_env, mock_func_map):

    services = {
        'servicea': {
            'image': '@{get_host_platform}:latest',
            'user': '@{get_user_id}:@{get_group_id}',
            'environment': {
                'COMPOSE_VERSION': '@{get_compose_version}',
                'DOCKER_VERSION': '@{get_docker_version}',
            }
        }
    }
    expected = {
        'servicea': {
            'image': 'mocked_platform:latest',
            'user': '5555:5556',
            'environment': {
                'COMPOSE_VERSION': compose.__version__,
                'DOCKER_VERSION': '1.2.3',
            }
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


def test_inihbate_invoke_funcmap_in_services(mock_env, mock_func_map):

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


def test_calling_unknown_helper_function(mock_env, mock_func_map):

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
