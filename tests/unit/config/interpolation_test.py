# encoding: utf-8
from __future__ import absolute_import
from __future__ import unicode_literals

import pytest

from compose.config.environment import Environment
from compose.config.errors import ConfigurationError
from compose.config.interpolation import interpolate_environment_variables
from compose.config.interpolation import Interpolator
from compose.config.interpolation import InvalidInterpolation
from compose.config.interpolation import TemplateWithDefaults
from compose.config.interpolation import UnsetRequiredSubstitution
from compose.const import COMPOSEFILE_V2_0 as V2_0
from compose.const import COMPOSEFILE_V2_3 as V2_3
from compose.const import COMPOSEFILE_V3_4 as V3_4


@pytest.fixture
def mock_env():
    return Environment({
        'USER': 'jenny',
        'FOO': 'bar',
        'TRUE': 'True',
        'FALSE': 'OFF',
        'POSINT': '50',
        'NEGINT': '-200',
        'FLOAT': '0.145',
        'MODE': '0600',
        'BYTES': '512m',
    })


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
                'max': '2',
                'user': 'jenny'
            }
        },
        'other': {},
    }
    value = interpolate_environment_variables(V3_4, secrets, 'secret', mock_env)
    assert value == expected


def test_interpolate_environment_services_convert_types_v2(mock_env):
    entry = {
        'service1': {
            'blkio_config': {
                'weight': '${POSINT}',
                'weight_device': [{'file': '/dev/sda1', 'weight': '${POSINT}'}]
            },
            'cpus': '${FLOAT}',
            'cpu_count': '$POSINT',
            'healthcheck': {
                'retries': '${POSINT:-3}',
                'disable': '${FALSE}',
                'command': 'true'
            },
            'mem_swappiness': '${DEFAULT:-127}',
            'oom_score_adj': '${NEGINT}',
            'scale': '${POSINT}',
            'ulimits': {
                'nproc': '${POSINT}',
                'nofile': {
                    'soft': '${POSINT}',
                    'hard': '${DEFAULT:-40000}'
                },
            },
            'privileged': '${TRUE}',
            'read_only': '${DEFAULT:-no}',
            'tty': '${DEFAULT:-N}',
            'stdin_open': '${DEFAULT-on}',
            'volumes': [
                {'type': 'tmpfs', 'target': '/target', 'tmpfs': {'size': '$BYTES'}}
            ]
        }
    }

    expected = {
        'service1': {
            'blkio_config': {
                'weight': 50,
                'weight_device': [{'file': '/dev/sda1', 'weight': 50}]
            },
            'cpus': 0.145,
            'cpu_count': 50,
            'healthcheck': {
                'retries': 50,
                'disable': False,
                'command': 'true'
            },
            'mem_swappiness': 127,
            'oom_score_adj': -200,
            'scale': 50,
            'ulimits': {
                'nproc': 50,
                'nofile': {
                    'soft': 50,
                    'hard': 40000
                },
            },
            'privileged': True,
            'read_only': False,
            'tty': False,
            'stdin_open': True,
            'volumes': [
                {'type': 'tmpfs', 'target': '/target', 'tmpfs': {'size': 536870912}}
            ]
        }
    }

    value = interpolate_environment_variables(V2_3, entry, 'service', mock_env)
    assert value == expected


def test_interpolate_environment_services_convert_types_v3(mock_env):
    entry = {
        'service1': {
            'healthcheck': {
                'retries': '${POSINT:-3}',
                'disable': '${FALSE}',
                'command': 'true'
            },
            'ulimits': {
                'nproc': '${POSINT}',
                'nofile': {
                    'soft': '${POSINT}',
                    'hard': '${DEFAULT:-40000}'
                },
            },
            'privileged': '${TRUE}',
            'read_only': '${DEFAULT:-no}',
            'tty': '${DEFAULT:-N}',
            'stdin_open': '${DEFAULT-on}',
            'deploy': {
                'update_config': {
                    'parallelism': '${DEFAULT:-2}',
                    'max_failure_ratio': '${FLOAT}',
                },
                'restart_policy': {
                    'max_attempts': '$POSINT',
                },
                'replicas': '${DEFAULT-3}'
            },
            'ports': [{'target': '${POSINT}', 'published': '${DEFAULT:-5000}'}],
            'configs': [{'mode': '${MODE}', 'source': 'config1'}],
            'secrets': [{'mode': '${MODE}', 'source': 'secret1'}],
        }
    }

    expected = {
        'service1': {
            'healthcheck': {
                'retries': 50,
                'disable': False,
                'command': 'true'
            },
            'ulimits': {
                'nproc': 50,
                'nofile': {
                    'soft': 50,
                    'hard': 40000
                },
            },
            'privileged': True,
            'read_only': False,
            'tty': False,
            'stdin_open': True,
            'deploy': {
                'update_config': {
                    'parallelism': 2,
                    'max_failure_ratio': 0.145,
                },
                'restart_policy': {
                    'max_attempts': 50,
                },
                'replicas': 3
            },
            'ports': [{'target': 50, 'published': 5000}],
            'configs': [{'mode': 0o600, 'source': 'config1'}],
            'secrets': [{'mode': 0o600, 'source': 'secret1'}],
        }
    }

    value = interpolate_environment_variables(V3_4, entry, 'service', mock_env)
    assert value == expected


def test_interpolate_environment_services_convert_types_invalid(mock_env):
    entry = {'service1': {'privileged': '${POSINT}'}}

    with pytest.raises(ConfigurationError) as exc:
        interpolate_environment_variables(V2_3, entry, 'service', mock_env)

    assert 'Error while attempting to convert service.service1.privileged to '\
        'appropriate type: "50" is not a valid boolean value' in exc.exconly()

    entry = {'service1': {'cpus': '${TRUE}'}}
    with pytest.raises(ConfigurationError) as exc:
        interpolate_environment_variables(V2_3, entry, 'service', mock_env)

    assert 'Error while attempting to convert service.service1.cpus to '\
        'appropriate type: "True" is not a valid float' in exc.exconly()

    entry = {'service1': {'ulimits': {'nproc': '${FLOAT}'}}}
    with pytest.raises(ConfigurationError) as exc:
        interpolate_environment_variables(V2_3, entry, 'service', mock_env)

    assert 'Error while attempting to convert service.service1.ulimits.nproc to '\
        'appropriate type: "0.145" is not a valid integer' in exc.exconly()


def test_interpolate_environment_network_convert_types(mock_env):
    entry = {
        'network1': {
            'external': '${FALSE}',
            'attachable': '${TRUE}',
            'internal': '${DEFAULT:-false}'
        }
    }

    expected = {
        'network1': {
            'external': False,
            'attachable': True,
            'internal': False,
        }
    }

    value = interpolate_environment_variables(V3_4, entry, 'network', mock_env)
    assert value == expected


def test_interpolate_environment_external_resource_convert_types(mock_env):
    entry = {
        'resource1': {
            'external': '${TRUE}',
        }
    }

    expected = {
        'resource1': {
            'external': True,
        }
    }

    value = interpolate_environment_variables(V3_4, entry, 'network', mock_env)
    assert value == expected
    value = interpolate_environment_variables(V3_4, entry, 'volume', mock_env)
    assert value == expected
    value = interpolate_environment_variables(V3_4, entry, 'secret', mock_env)
    assert value == expected
    value = interpolate_environment_variables(V3_4, entry, 'config', mock_env)
    assert value == expected


def test_interpolate_service_name_uses_dot(mock_env):
    entry = {
        'service.1': {
            'image': 'busybox',
            'ulimits': {
                'nproc': '${POSINT}',
                'nofile': {
                    'soft': '${POSINT}',
                    'hard': '${DEFAULT:-40000}'
                },
            },
        }
    }

    expected = {
        'service.1': {
            'image': 'busybox',
            'ulimits': {
                'nproc': 50,
                'nofile': {
                    'soft': 50,
                    'hard': 40000
                },
            },
        }
    }

    value = interpolate_environment_variables(V3_4, entry, 'service', mock_env)
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


def test_interpolate_with_empty_and_default_value(defaults_interpolator):
    assert defaults_interpolator("ok ${BAR:-def}") == "ok def"
    assert defaults_interpolator("ok ${BAR-def}") == "ok "


def test_interpolate_mandatory_values(defaults_interpolator):
    assert defaults_interpolator("ok ${FOO:?bar}") == "ok first"
    assert defaults_interpolator("ok ${FOO?bar}") == "ok first"
    assert defaults_interpolator("ok ${BAR?bar}") == "ok "

    with pytest.raises(UnsetRequiredSubstitution) as e:
        defaults_interpolator("not ok ${BAR:?high bar}")
    assert e.value.err == 'high bar'

    with pytest.raises(UnsetRequiredSubstitution) as e:
        defaults_interpolator("not ok ${BAZ?dropped the bazz}")
    assert e.value.err == 'dropped the bazz'


def test_interpolate_mandatory_no_err_msg(defaults_interpolator):
    with pytest.raises(UnsetRequiredSubstitution) as e:
        defaults_interpolator("not ok ${BAZ?}")

    assert e.value.err == ''


def test_interpolate_mixed_separators(defaults_interpolator):
    assert defaults_interpolator("ok ${BAR:-/non:-alphanumeric}") == "ok /non:-alphanumeric"
    assert defaults_interpolator("ok ${BAR:-:?wwegegr??:?}") == "ok :?wwegegr??:?"
    assert defaults_interpolator("ok ${BAR-:-hello}") == 'ok '

    with pytest.raises(UnsetRequiredSubstitution) as e:
        defaults_interpolator("not ok ${BAR:?xazz:-redf}")
    assert e.value.err == 'xazz:-redf'

    assert defaults_interpolator("ok ${BAR?...:?bar}") == "ok "


def test_unbraced_separators(defaults_interpolator):
    assert defaults_interpolator("ok $FOO:-bar") == "ok first:-bar"
    assert defaults_interpolator("ok $BAZ?error") == "ok ?error"


def test_interpolate_unicode_values():
    variable_mapping = {
        'FOO': '十六夜　咲夜'.encode('utf-8'),
        'BAR': '十六夜　咲夜'
    }
    interpol = Interpolator(TemplateWithDefaults, variable_mapping).interpolate

    interpol("$FOO") == '十六夜　咲夜'
    interpol("${BAR}") == '十六夜　咲夜'


def test_interpolate_no_fallthrough():
    # Test regression on docker/compose#5829
    variable_mapping = {
        'TEST:-': 'hello',
        'TEST-': 'hello',
    }
    interpol = Interpolator(TemplateWithDefaults, variable_mapping).interpolate

    assert interpol('${TEST:-}') == ''
    assert interpol('${TEST-}') == ''
