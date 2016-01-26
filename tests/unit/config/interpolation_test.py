from __future__ import absolute_import
from __future__ import unicode_literals

import os

import mock
import pytest

from compose.config.interpolation import interpolate_environment_variables
from compose.config.validation import load_jsonschema


@pytest.yield_fixture
def mock_env():
    with mock.patch.dict(os.environ):
        os.environ['USER'] = 'jenny'
        os.environ['FOO'] = 'bar'
        os.environ['RO'] = 'false'
        os.environ['VOLUMES'] = "['/tmp/bar:/bar', '/tmp/foo:/foo']"
        os.environ['CPU_SHARES'] = '512'
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
    assert interpolate_environment_variables(services, 'service', load_jsonschema('service', 2)) == expected


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
    assert interpolate_environment_variables(services, 'service', load_jsonschema('service', 2)) == expected


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
    assert interpolate_environment_variables(volumes, 'volume', load_jsonschema('service', 2)) == expected
