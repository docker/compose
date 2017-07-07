from __future__ import absolute_import
from __future__ import unicode_literals

import pytest

from compose.config.environment import Environment
from compose.config.interpolation import interpolate_environment_variables
from compose.config.interpolation import Interpolator
from compose.config.interpolation import InvalidInterpolation
from compose.config.interpolation import TemplateWithDefaults
from compose.const import COMPOSEFILE_V2_0 as V2_0
from compose.const import COMPOSEFILE_V3_1 as V3_1


@pytest.fixture
def mock_env():
    return Environment({'USER': 'jenny', 'FOO': 'bar'})


@pytest.fixture
def variable_mapping():
    return Environment({'FOO': 'first', 'BAR': ''})


@pytest.fixture
def defaults_interpolator(variable_mapping):
    return Interpolator(TemplateWithDefaults, variable_mapping).interpolate


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
    value = interpolate_environment_variables(V2_0, services, 'service', mock_env)
    assert value == expected


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
    value = interpolate_environment_variables(V2_0, volumes, 'volume', mock_env)
    assert value == expected


def test_interpolate_environment_variables_in_secrets(mock_env):
    secrets = {
        'secretservice': {
            'file': '$FOO',
            'labels': {
                'max': 2,
                'user': '${USER}'
            }
        },
        'other': None,
    }
    expected = {
        'secretservice': {
            'file': 'bar',
            'labels': {
                'max': 2,
                'user': 'jenny'
            }
        },
        'other': {},
    }
    value = interpolate_environment_variables(V3_1, secrets, 'volume', mock_env)
    assert value == expected


def test_escaped_interpolation(defaults_interpolator):
    assert defaults_interpolator('$${foo}') == '${foo}'


def test_invalid_interpolation(defaults_interpolator):
    with pytest.raises(InvalidInterpolation):
        defaults_interpolator('${')
    with pytest.raises(InvalidInterpolation):
        defaults_interpolator('$}')
    with pytest.raises(InvalidInterpolation):
        defaults_interpolator('${}')
    with pytest.raises(InvalidInterpolation):
        defaults_interpolator('${ }')
    with pytest.raises(InvalidInterpolation):
        defaults_interpolator('${ foo}')
    with pytest.raises(InvalidInterpolation):
        defaults_interpolator('${foo }')
    with pytest.raises(InvalidInterpolation):
        defaults_interpolator('${foo!}')


def test_interpolate_missing_no_default(defaults_interpolator):
    assert defaults_interpolator("This ${missing} var") == "This  var"
    assert defaults_interpolator("This ${BAR} var") == "This  var"


def test_interpolate_with_value(defaults_interpolator):
    assert defaults_interpolator("This $FOO var") == "This first var"
    assert defaults_interpolator("This ${FOO} var") == "This first var"


def test_interpolate_missing_with_default(defaults_interpolator):
    assert defaults_interpolator("ok ${missing:-def}") == "ok def"
    assert defaults_interpolator("ok ${missing-def}") == "ok def"
    assert defaults_interpolator("ok ${BAR:-/non:-alphanumeric}") == "ok /non:-alphanumeric"


def test_interpolate_with_empty_and_default_value(defaults_interpolator):
    assert defaults_interpolator("ok ${BAR:-def}") == "ok def"
    assert defaults_interpolator("ok ${BAR-def}") == "ok "
