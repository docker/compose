from __future__ import absolute_import
from __future__ import unicode_literals

import os

import mock
import pytest

from compose.config.interpolation import interpolate_environment_variables


@pytest.yield_fixture
def mock_env():
    with mock.patch.dict(os.environ):
        os.environ['USER'] = 'jenny'
        os.environ['FOO'] = 'bar'
        yield


def test_interpolate_environment_variables_in_services(mock_env):
    services = {
        'servivea': {
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
        'servivea': {
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
    assert interpolate_environment_variables(services, 'service') == expected


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
    assert interpolate_environment_variables(volumes, 'volume') == expected
