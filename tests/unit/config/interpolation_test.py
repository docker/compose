from __future__ import absolute_import
from __future__ import unicode_literals

import os

import mock
import pytest

from compose.config.interpolation import interpolate_environment_variables
from compose.config.validation import load_jsonschema
from compose.const import COMPOSEFILE_V2_0 as V2_0


@pytest.yield_fixture
def mock_env():
    with mock.patch.dict(os.environ):
        os.environ['USER'] = 'jenny'
        os.environ['FOO'] = 'bar'
        os.environ['RO'] = 'false'
        os.environ['VOLUMES'] = "['/tmp/bar:/bar', '/tmp/foo:/foo']"
        os.environ['VOLUMES_ITEM'] = '/tmp/foo:/foo'
        os.environ['CPU_SHARES'] = '512'
        os.environ['PORTS'] = "[ 8080 , 8089 ]"
        os.environ['SINGLE_PORT'] = '8787'
        yield


def test_interpolate_environment_variables_in_services(mock_env):
    services = {
        'servivea': {
            'image': 'example:${USER}',
            'entrypoint': '/bin/bash',
            'volumes': ['$FOO:/target'],
            'read_only': '${RO}',
            'cpu_shares': '${CPU_SHARES}',
            'logging': {
                'driver': '${FOO}',
                'options': {
                    'user': '$USER',
                }
            }
        }
    }
    expected = {
        'servivea': {
            'image': 'example:jenny',
            'entrypoint': '/bin/bash',
            'volumes': ['bar:/target'],
            'read_only': False,
            'cpu_shares': 512,
            'logging': {
                'driver': 'bar',
                'options': {
                    'user': 'jenny',
                }
            }
        }
    }
    assert interpolate_environment_variables(services, 'service', load_jsonschema('service', V2_0)) == expected


def test_interpolate_environment_variables_arrays_in_services(mock_env):
    services = {
        'servivea': {
            'image': 'example:${USER}',
            'volumes': '${VOLUMES}',
            'cpu_shares': '${CPU_SHARES}',
            }
        }
    expected = {
        'servivea': {
            'image': 'example:jenny',
            'volumes': ['/tmp/bar:/bar', '/tmp/foo:/foo'],
            'cpu_shares': 512,
            }
        }
    assert interpolate_environment_variables(services, 'service', load_jsonschema('service', V2_0)) == expected


def test_interpolate_environment_variables_array_element_in_services(mock_env):
    services = {
        'servivea': {
            'image': 'example:${USER}',
            'volumes': ['/tmp/bar:/bar', '${VOLUMES_ITEM}'],
            'cpu_shares': '${CPU_SHARES}',
            }
        }
    expected = {
        'servivea': {
            'image': 'example:jenny',
            'volumes': ['/tmp/bar:/bar', '/tmp/foo:/foo'],
            'cpu_shares': 512,
            }
        }
    assert interpolate_environment_variables(services, 'service', load_jsonschema('service', V2_0)) == expected


def test_interpolate_environment_variables_array_numbers_in_services(mock_env):
    services = {
        'servivea': {
            'expose': '${PORTS}',
            'ports': [8080, '${SINGLE_PORT}']
            }
        }
    expected = {
        'servivea': {
            'expose': [8080, 8089],
            'ports': [8080, 8787]
            }
        }
    assert interpolate_environment_variables(services, 'service', load_jsonschema('service', V2_0)) == expected


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
    assert interpolate_environment_variables(volumes, 'volume', load_jsonschema('service', V2_0)) == expected
