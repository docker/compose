import codecs
import os
import shutil
import tempfile
from operator import itemgetter
from random import shuffle

import pytest
import yaml
from ddt import data
from ddt import ddt

from ...helpers import build_config_details
from ...helpers import BUSYBOX_IMAGE_WITH_TAG
from ...helpers import cd
from compose.config import config
from compose.config import types
from compose.config.config import ConfigFile
from compose.config.config import resolve_build_args
from compose.config.config import resolve_environment
from compose.config.environment import Environment
from compose.config.errors import ConfigurationError
from compose.config.errors import VERSION_EXPLANATION
from compose.config.serialize import denormalize_service_dict
from compose.config.serialize import serialize_config
from compose.config.serialize import serialize_ns_time_value
from compose.config.types import VolumeSpec
from compose.const import COMPOSE_SPEC as VERSION
from compose.const import COMPOSEFILE_V1 as V1
from compose.const import IS_WINDOWS_PLATFORM
from tests import mock
from tests import unittest

DEFAULT_VERSION = VERSION


def make_service_dict(name, service_dict, working_dir='.', filename=None):
    """Test helper function to construct a ServiceExtendsResolver
    """
    resolver = config.ServiceExtendsResolver(
        config.ServiceConfig(
            working_dir=working_dir,
            filename=filename,
            name=name,
            config=service_dict),
        config.ConfigFile(filename=filename, config={}),
        environment=Environment.from_env_file(working_dir)
    )
    return config.process_service(resolver.run())


def service_sort(services):
    return sorted(services, key=itemgetter('name'))


def secret_sort(secrets):
    return sorted(secrets, key=itemgetter('source'))


@ddt
class ConfigTest(unittest.TestCase):

    def test_load(self):
        service_dicts = config.load(
            build_config_details(
                {
                    'services': {
                        'foo': {'image': 'busybox'},
                        'bar': {'image': 'busybox', 'environment': ['FOO=1']},
                    }
                },
                'tests/fixtures/extends',
                'common.yml'
            )
        ).services

        assert service_sort(service_dicts) == service_sort([
            {
                'name': 'bar',
                'image': 'busybox',
                'environment': {'FOO': '1'},
            },
            {
                'name': 'foo',
                'image': 'busybox',
            }
        ])

    def test_load_v2(self):
        config_data = config.load(
            build_config_details({
                'version': '2',
                'services': {
                    'foo': {'image': 'busybox'},
                    'bar': {'image': 'busybox', 'environment': ['FOO=1']},
                },
                'volumes': {
                    'hello': {
                        'driver': 'default',
                        'driver_opts': {'beep': 'boop'}
                    }
                },
                'networks': {
                    'default': {
                        'driver': 'bridge',
                        'driver_opts': {'beep': 'boop'}
                    },
                    'with_ipam': {
                        'ipam': {
                            'driver': 'default',
                            'config': [
                                {'subnet': '172.28.0.0/16'}
                            ]
                        }
                    },
                    'internal': {
                        'driver': 'bridge',
                        'internal': True
                    }
                }
            }, 'working_dir', 'filename.yml')
        )
        service_dicts = config_data.services
        volume_dict = config_data.volumes
        networks_dict = config_data.networks
        assert service_sort(service_dicts) == service_sort([
            {
                'name': 'bar',
                'image': 'busybox',
                'environment': {'FOO': '1'},
            },
            {
                'name': 'foo',
                'image': 'busybox',
            }
        ])
        assert volume_dict == {
            'hello': {
                'driver': 'default',
                'driver_opts': {'beep': 'boop'}
            }
        }
        assert networks_dict == {
            'default': {
                'driver': 'bridge',
                'driver_opts': {'beep': 'boop'}
            },
            'with_ipam': {
                'ipam': {
                    'driver': 'default',
                    'config': [
                        {'subnet': '172.28.0.0/16'}
                    ]
                }
            },
            'internal': {
                'driver': 'bridge',
                'internal': True
            }
        }

    def test_valid_versions(self):
        cfg = config.load(
            build_config_details({
                'services': {
                    'foo': {'image': 'busybox'},
                    'bar': {'image': 'busybox', 'environment': ['FOO=1']},
                }
            })
        )
        assert cfg.config_version == VERSION
        assert cfg.version == VERSION

        for version in ['2', '2.0', '2.1', '2.2', '2.3',
                        '3', '3.0', '3.1', '3.2', '3.3', '3.4', '3.5', '3.6', '3.7', '3.8']:
            cfg = config.load(build_config_details({'version': version}))
            assert cfg.config_version == version
            assert cfg.version == VERSION

    def test_v1_file_version(self):
        cfg = config.load(build_config_details({'web': {'image': 'busybox'}}))
        assert cfg.version == V1
        assert list(s['name'] for s in cfg.services) == ['web']

        cfg = config.load(build_config_details({'version': {'image': 'busybox'}}))
        assert cfg.version == V1
        assert list(s['name'] for s in cfg.services) == ['version']

    def test_wrong_version_type(self):
        for version in [1, 2, 2.0]:
            with pytest.raises(ConfigurationError) as excinfo:
                config.load(
                    build_config_details(
                        {'version': version},
                        filename='filename.yml',
                    )
                )

            assert 'Version in "filename.yml" is invalid - it should be a string.' \
                in excinfo.exconly()

    def test_unsupported_version(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {'version': '1'},
                    filename='filename.yml',
                )
            )

        assert 'Version in "filename.yml" is invalid' in excinfo.exconly()
        assert VERSION_EXPLANATION in excinfo.exconly()

    def test_version_1_is_invalid(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': '1',
                        'web': {'image': 'busybox'},
                    },
                    filename='filename.yml',
                )
            )

        assert 'Version in "filename.yml" is invalid' in excinfo.exconly()
        assert VERSION_EXPLANATION in excinfo.exconly()

    def test_v1_file_with_version_is_invalid(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': '2',
                        'web': {'image': 'busybox'},
                    },
                    filename='filename.yml',
                )
            )

        assert "compose.config.errors.ConfigurationError: " \
               "The Compose file 'filename.yml' is invalid because:\n" \
               "'web' does not match any of the regexes: '^x-'" in excinfo.exconly()
        assert VERSION_EXPLANATION in excinfo.exconly()

    def test_named_volume_config_empty(self):
        config_details = build_config_details({
            'version': '2',
            'services': {
                'simple': {'image': 'busybox'}
            },
            'volumes': {
                'simple': None,
                'other': {},
            }
        })
        config_result = config.load(config_details)
        volumes = config_result.volumes
        assert 'simple' in volumes
        assert volumes['simple'] == {}
        assert volumes['other'] == {}

    def test_named_volume_numeric_driver_opt(self):
        config_details = build_config_details({
            'version': '2',
            'services': {
                'simple': {'image': 'busybox'}
            },
            'volumes': {
                'simple': {'driver_opts': {'size': 42}},
            }
        })
        cfg = config.load(config_details)
        assert cfg.volumes['simple']['driver_opts']['size'] == '42'

    def test_volume_invalid_driver_opt(self):
        config_details = build_config_details({
            'version': '2',
            'services': {
                'simple': {'image': 'busybox'}
            },
            'volumes': {
                'simple': {'driver_opts': {'size': True}},
            }
        })
        with pytest.raises(ConfigurationError) as exc:
            config.load(config_details)
        assert 'driver_opts.size contains an invalid type' in exc.exconly()

    def test_named_volume_invalid_type_list(self):
        config_details = build_config_details({
            'version': '2',
            'services': {
                'simple': {'image': 'busybox'}
            },
            'volumes': []
        })
        with pytest.raises(ConfigurationError) as exc:
            config.load(config_details)
        assert "volume must be a mapping, not an array" in exc.exconly()

    def test_networks_invalid_type_list(self):
        config_details = build_config_details({
            'version': '2',
            'services': {
                'simple': {'image': 'busybox'}
            },
            'networks': []
        })
        with pytest.raises(ConfigurationError) as exc:
            config.load(config_details)
        assert "network must be a mapping, not an array" in exc.exconly()

    def test_load_service_with_name_version(self):
        with mock.patch('compose.config.config.log') as mock_logging:
            config_data = config.load(
                build_config_details({
                    'version': {
                        'image': 'busybox'
                    }
                }, 'working_dir', 'filename.yml')
            )
        assert 'Unexpected type for "version" key in "filename.yml"' \
            in mock_logging.warning.call_args[0][0]

        service_dicts = config_data.services
        assert service_sort(service_dicts) == service_sort([
            {
                'name': 'version',
                'image': 'busybox',
            }
        ])

    def test_load_throws_error_when_not_dict(self):
        with pytest.raises(ConfigurationError):
            config.load(
                build_config_details(
                    {'web': BUSYBOX_IMAGE_WITH_TAG},
                    'working_dir',
                    'filename.yml'
                )
            )

    def test_load_throws_error_when_not_dict_v2(self):
        with pytest.raises(ConfigurationError):
            config.load(
                build_config_details(
                    {'version': '2', 'services': {'web': BUSYBOX_IMAGE_WITH_TAG}},
                    'working_dir',
                    'filename.yml'
                )
            )

    def test_load_throws_error_with_invalid_network_fields(self):
        with pytest.raises(ConfigurationError):
            config.load(
                build_config_details({
                    'version': '2',
                    'services': {'web': BUSYBOX_IMAGE_WITH_TAG},
                    'networks': {
                        'invalid': {'foo', 'bar'}
                    }
                }, 'working_dir', 'filename.yml')
            )

    def test_load_config_link_local_ips_network(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '2',
                'services': {
                    'web': {
                        'image': 'example/web',
                        'networks': {
                            'foobar': {
                                'aliases': ['foo', 'bar'],
                                'link_local_ips': ['169.254.8.8']
                            }
                        }
                    }
                },
                'networks': {'foobar': {}}
            }
        )

        details = config.ConfigDetails('.', [base_file])
        web_service = config.load(details).services[0]
        assert web_service['networks'] == {
            'foobar': {
                'aliases': ['foo', 'bar'],
                'link_local_ips': ['169.254.8.8']
            }
        }

    def test_load_config_service_labels(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '2.1',
                'services': {
                    'web': {
                        'image': 'example/web',
                        'labels': ['label_key=label_val']
                    },
                    'db': {
                        'image': 'example/db',
                        'labels': {
                            'label_key': 'label_val'
                        }
                    }
                },
            }
        )
        details = config.ConfigDetails('.', [base_file])
        service_dicts = config.load(details).services
        for service in service_dicts:
            assert service['labels'] == {
                'label_key': 'label_val'
            }

    def test_load_config_custom_resource_names(self):
        base_file = config.ConfigFile(
            'base.yaml', {
                'version': '3.5',
                'volumes': {
                    'abc': {
                        'name': 'xyz'
                    }
                },
                'networks': {
                    'abc': {
                        'name': 'xyz'
                    }
                },
                'secrets': {
                    'abc': {
                        'name': 'xyz'
                    }
                },
                'configs': {
                    'abc': {
                        'name': 'xyz'
                    }
                }
            }
        )
        details = config.ConfigDetails('.', [base_file])
        loaded_config = config.load(details)

        assert loaded_config.networks['abc'] == {'name': 'xyz'}
        assert loaded_config.volumes['abc'] == {'name': 'xyz'}
        assert loaded_config.secrets['abc']['name'] == 'xyz'
        assert loaded_config.configs['abc']['name'] == 'xyz'

    def test_load_config_volume_and_network_labels(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '2.1',
                'services': {
                    'web': {
                        'image': 'example/web',
                    },
                },
                'networks': {
                    'with_label': {
                        'labels': {
                            'label_key': 'label_val'
                        }
                    }
                },
                'volumes': {
                    'with_label': {
                        'labels': {
                            'label_key': 'label_val'
                        }
                    }
                }
            }
        )

        details = config.ConfigDetails('.', [base_file])
        loaded_config = config.load(details)

        assert loaded_config.networks == {
            'with_label': {
                'labels': {
                    'label_key': 'label_val'
                }
            }
        }

        assert loaded_config.volumes == {
            'with_label': {
                'labels': {
                    'label_key': 'label_val'
                }
            }
        }

    def test_load_config_invalid_service_names(self):
        for invalid_name in ['?not?allowed', ' ', '', '!', '/', '\xe2']:
            with pytest.raises(ConfigurationError) as exc:
                config.load(build_config_details(
                    {
                        'version': '2',
                        'services': {
                            invalid_name:
                            {
                                'image': 'busybox'
                            }
                        }
                    }))
            assert 'Invalid service name \'%s\'' % invalid_name in exc.exconly()

    def test_load_config_invalid_service_names_v2(self):
        for invalid_name in ['?not?allowed', ' ', '', '!', '/', '\xe2']:
            with pytest.raises(ConfigurationError) as exc:
                config.load(build_config_details(
                    {
                        'version': '2',
                        'services': {invalid_name: {'image': 'busybox'}},
                    }))
            assert 'Invalid service name \'%s\'' % invalid_name in exc.exconly()

    def test_load_with_invalid_field_name(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details(
                {
                    'version': '2',
                    'services': {
                        'web': {'image': 'busybox', 'name': 'bogus'},
                    }
                },
                'working_dir',
                'filename.yml',
            ))

        assert "Unsupported config option for services.web: 'name'" in exc.exconly()

    def test_load_with_invalid_field_name_v1(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details(
                {
                    'version': '2',
                    'services': {
                        'web': {'image': 'busybox', 'name': 'bogus'}
                    }
                },
                'working_dir',
                'filename.yml',
            ))
        assert "Unsupported config option for services.web: 'name'" in exc.exconly()

    def test_load_invalid_service_definition(self):
        config_details = build_config_details(
            {
                'version': '2',
                'services': {
                    'web': 'wrong'
                }
            },
            'working_dir',
            'filename.yml')
        with pytest.raises(ConfigurationError) as exc:
            config.load(config_details)
        assert "service 'web' must be a mapping not a string." in exc.exconly()

    def test_load_with_empty_build_args(self):
        config_details = build_config_details(
            {
                'version': '2',
                'services': {
                    'web': {
                        'build': {
                            'context': os.getcwd(),
                            'args': None,
                        },
                    },
                },
            }
        )
        with pytest.raises(ConfigurationError) as exc:
            config.load(config_details)
        assert (
            "services.web.build.args contains an invalid type, it should be an "
            "object, or an array" in exc.exconly()
        )

    def test_config_integer_service_name_raise_validation_error(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': '2',
                        'services': {1: {'image': 'busybox'}}
                    },
                    'working_dir',
                    'filename.yml'
                )
            )

        assert (
            "In file 'filename.yml', the service name 1 must be a quoted string, i.e. '1'" in
            excinfo.exconly()
        )

    def test_config_integer_service_name_raise_validation_error_v2(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': '2',
                        'services': {1: {'image': 'busybox'}}
                    },
                    'working_dir',
                    'filename.yml'
                )
            )

        assert (
            "In file 'filename.yml', the service name 1 must be a quoted string, i.e. '1'." in
            excinfo.exconly()
        )

    def test_config_integer_service_name_raise_validation_error_v2_when_no_interpolate(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': '2',
                        'services': {1: {'image': 'busybox'}}
                    },
                    'working_dir',
                    'filename.yml'
                ),
                interpolate=False
            )

        assert (
            "In file 'filename.yml', the service name 1 must be a quoted string, i.e. '1'." in
            excinfo.exconly()
        )

    def test_config_integer_service_property_raise_validation_error(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details({
                    'version': '2.1',
                    'services': {'foobar': {'image': 'busybox', 1234: 'hah'}}
                }, 'working_dir', 'filename.yml')
            )

        assert (
            "Unsupported config option for services.foobar: '1234'" in excinfo.exconly()
        )

    def test_config_invalid_service_name_raise_validation_error(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details({
                    'version': '2',
                    'services': {
                        'test_app': {'build': '.'},
                        'mong\\o': {'image': 'mongo'},
                    }
                })
            )

            assert 'Invalid service name \'mong\\o\'' in excinfo.exconly()

    def test_config_duplicate_cache_from_values_no_validation_error(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(
                build_config_details({
                    'version': '2.3',
                    'services': {
                        'test': {'build': {'context': '.', 'cache_from': ['a', 'b', 'a']}}
                    }

                })
            )

        assert 'build.cache_from contains non-unique items' not in exc.exconly()

    def test_load_with_multiple_files_v1(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'web': {
                    'image': 'example/web',
                    'links': ['db'],
                },
                'db': {
                    'image': 'example/db',
                },
            })
        override_file = config.ConfigFile(
            'override.yaml',
            {
                'web': {
                    'build': '/',
                    'volumes': ['/home/user/project:/code'],
                },
            })
        details = config.ConfigDetails('.', [base_file, override_file])

        service_dicts = config.load(details).services
        expected = [
            {
                'name': 'web',
                'build': {'context': os.path.abspath('/')},
                'volumes': [VolumeSpec.parse('/home/user/project:/code')],
                'links': ['db'],
            },
            {
                'name': 'db',
                'image': 'example/db',
            },
        ]
        assert service_sort(service_dicts) == service_sort(expected)

    def test_load_with_multiple_files_and_empty_override(self):
        base_file = config.ConfigFile(
            'base.yml',
            {'web': {'image': 'example/web'}})
        override_file = config.ConfigFile('override.yml', None)
        details = config.ConfigDetails('.', [base_file, override_file])

        with pytest.raises(ConfigurationError) as exc:
            config.load(details)
        error_msg = "Top level object in 'override.yml' needs to be an object"
        assert error_msg in exc.exconly()

    def test_load_with_multiple_files_and_empty_override_v2(self):
        base_file = config.ConfigFile(
            'base.yml',
            {'version': '2', 'services': {'web': {'image': 'example/web'}}})
        override_file = config.ConfigFile('override.yml', None)
        details = config.ConfigDetails('.', [base_file, override_file])

        with pytest.raises(ConfigurationError) as exc:
            config.load(details)
        error_msg = "Top level object in 'override.yml' needs to be an object"
        assert error_msg in exc.exconly()

    def test_load_with_multiple_files_and_empty_base(self):
        base_file = config.ConfigFile('base.yml', None)
        override_file = config.ConfigFile(
            'override.yml',
            {'web': {'image': 'example/web'}})
        details = config.ConfigDetails('.', [base_file, override_file])

        with pytest.raises(ConfigurationError) as exc:
            config.load(details)
        assert "Top level object in 'base.yml' needs to be an object" in exc.exconly()

    def test_load_with_multiple_files_and_empty_base_v2(self):
        base_file = config.ConfigFile('base.yml', None)
        override_file = config.ConfigFile(
            'override.tml',
            {'version': '2', 'services': {'web': {'image': 'example/web'}}}
        )
        details = config.ConfigDetails('.', [base_file, override_file])
        with pytest.raises(ConfigurationError) as exc:
            config.load(details)
        assert "Top level object in 'base.yml' needs to be an object" in exc.exconly()

    def test_load_with_multiple_files_and_extends_in_override_file(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'web': {'image': 'example/web'},
            })
        override_file = config.ConfigFile(
            'override.yaml',
            {
                'web': {
                    'extends': {
                        'file': 'common.yml',
                        'service': 'base',
                    },
                    'volumes': ['/home/user/project:/code'],
                },
            })
        details = config.ConfigDetails('.', [base_file, override_file])

        tmpdir = tempfile.mkdtemp('config_test')
        self.addCleanup(shutil.rmtree, tmpdir)
        with open(os.path.join(tmpdir, 'common.yml'), mode="w") as common_fh:
            common_fh.write("""
                base:
                  labels: ['label=one']
            """)
        with cd(tmpdir):
            service_dicts = config.load(details).services

        expected = [
            {
                'name': 'web',
                'image': 'example/web',
                'volumes': [VolumeSpec.parse('/home/user/project:/code')],
                'labels': {'label': 'one'},
            },
        ]
        assert service_sort(service_dicts) == service_sort(expected)

    def test_load_mixed_extends_resolution(self):
        main_file = config.ConfigFile(
            'main.yml', {
                'version': '2.2',
                'services': {
                    'prodweb': {
                        'extends': {
                            'service': 'web',
                            'file': 'base.yml'
                        },
                        'environment': {'PROD': 'true'},
                    },
                },
            }
        )

        tmpdir = tempfile.mkdtemp('config_test')
        self.addCleanup(shutil.rmtree, tmpdir)
        with open(os.path.join(tmpdir, 'base.yml'), mode="w") as base_fh:
            base_fh.write("""
                version: '2.2'
                services:
                  base:
                    image: base
                  web:
                    extends: base
            """)

        details = config.ConfigDetails('.', [main_file])
        with cd(tmpdir):
            service_dicts = config.load(details).services
            assert service_dicts[0] == {
                'name': 'prodweb',
                'image': 'base',
                'environment': {'PROD': 'true'},
            }

    def test_load_with_multiple_files_and_invalid_override(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {'version': '2', 'services': {'web': {'image': 'example/web'}}})
        override_file = config.ConfigFile(
            'override.yaml',
            {'version': '2', 'services': {'bogus': 'thing'}})
        details = config.ConfigDetails('.', [base_file, override_file])

        with pytest.raises(ConfigurationError) as exc:
            config.load(details)
        assert "service 'bogus' must be a mapping not a string." in exc.exconly()
        assert "In file 'override.yaml'" in exc.exconly()

    def test_load_sorts_in_dependency_order(self):
        config_details = build_config_details({
            'web': {
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'links': ['db'],
            },
            'db': {
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'volumes_from': ['volume:ro']
            },
            'volume': {
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'volumes': ['/tmp'],
            }
        })
        services = config.load(config_details).services

        assert services[0]['name'] == 'volume'
        assert services[1]['name'] == 'db'
        assert services[2]['name'] == 'web'

    def test_load_with_extensions(self):
        config_details = build_config_details({
            'version': '2.3',
            'x-data': {
                'lambda': 3,
                'excess': [True, {}]
            }
        })

        config_data = config.load(config_details)
        assert config_data.services == []

    def test_config_build_configuration(self):
        service = config.load(
            build_config_details(
                {'web': {
                    'build': '.',
                    'dockerfile': 'Dockerfile-alt'
                }},
                'tests/fixtures/extends',
                'filename.yml'
            )
        ).services
        assert 'context' in service[0]['build']
        assert service[0]['build']['dockerfile'] == 'Dockerfile-alt'

    def test_config_build_configuration_v2(self):
        # service.dockerfile is invalid in v2
        with pytest.raises(ConfigurationError):
            config.load(
                build_config_details(
                    {
                        'version': '2',
                        'services': {
                            'web': {
                                'build': '.',
                                'dockerfile': 'Dockerfile-alt'
                            }
                        }
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        service = config.load(
            build_config_details({
                'version': '2',
                'services': {
                    'web': {
                        'build': '.'
                    }
                }
            }, 'tests/fixtures/extends', 'filename.yml')
        ).services[0]
        assert 'context' in service['build']

        service = config.load(
            build_config_details(
                {
                    'version': '2',
                    'services': {
                        'web': {
                            'build': {
                                'context': '.',
                                'dockerfile': 'Dockerfile-alt'
                            }
                        }
                    }
                },
                'tests/fixtures/extends',
                'filename.yml'
            )
        ).services
        assert 'context' in service[0]['build']
        assert service[0]['build']['dockerfile'] == 'Dockerfile-alt'

    def test_load_with_buildargs(self):
        service = config.load(
            build_config_details(
                {
                    'version': '2',
                    'services': {
                        'web': {
                            'build': {
                                'context': '.',
                                'dockerfile': 'Dockerfile-alt',
                                'args': {
                                    'opt1': 42,
                                    'opt2': 'foobar'
                                }
                            }
                        }
                    }
                },
                'tests/fixtures/extends',
                'filename.yml'
            )
        ).services[0]
        assert 'args' in service['build']
        assert 'opt1' in service['build']['args']
        assert isinstance(service['build']['args']['opt1'], str)
        assert service['build']['args']['opt1'] == '42'
        assert service['build']['args']['opt2'] == 'foobar'

    def test_load_build_labels_dict(self):
        service = config.load(
            build_config_details(
                {
                    'services': {
                        'web': {
                            'build': {
                                'context': '.',
                                'dockerfile': 'Dockerfile-alt',
                                'labels': {
                                    'label1': 42,
                                    'label2': 'foobar'
                                }
                            }
                        }
                    }
                },
                'tests/fixtures/extends',
                'filename.yml'
            )
        ).services[0]
        assert 'labels' in service['build']
        assert 'label1' in service['build']['labels']
        assert service['build']['labels']['label1'] == '42'
        assert service['build']['labels']['label2'] == 'foobar'

    def test_load_build_labels_list(self):
        base_file = config.ConfigFile(
            'base.yml',
            {
                'version': '2.3',
                'services': {
                    'web': {
                        'build': {
                            'context': '.',
                            'labels': ['foo=bar', 'baz=true', 'foobar=1']
                        },
                    },
                },
            }
        )

        details = config.ConfigDetails('.', [base_file])
        service = config.load(details).services[0]
        assert service['build']['labels'] == {
            'foo': 'bar', 'baz': 'true', 'foobar': '1'
        }

    def test_build_args_allow_empty_properties(self):
        service = config.load(
            build_config_details(
                {
                    'version': '2',
                    'services': {
                        'web': {
                            'build': {
                                'context': '.',
                                'dockerfile': 'Dockerfile-alt',
                                'args': {
                                    'foo': None
                                }
                            }
                        }
                    }
                },
                'tests/fixtures/extends',
                'filename.yml'
            )
        ).services[0]
        assert 'args' in service['build']
        assert 'foo' in service['build']['args']
        assert service['build']['args']['foo'] == ''

    # If build argument is None then it will be converted to the empty
    # string. Make sure that int zero kept as it is, i.e. not converted to
    # the empty string
    def test_build_args_check_zero_preserved(self):
        service = config.load(
            build_config_details(
                {
                    'version': '2',
                    'services': {
                        'web': {
                            'build': {
                                'context': '.',
                                'dockerfile': 'Dockerfile-alt',
                                'args': {
                                    'foo': 0
                                }
                            }
                        }
                    }
                },
                'tests/fixtures/extends',
                'filename.yml'
            )
        ).services[0]
        assert 'args' in service['build']
        assert 'foo' in service['build']['args']
        assert service['build']['args']['foo'] == '0'

    def test_load_with_multiple_files_mismatched_networks_format(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '2',
                'services': {
                    'web': {
                        'image': 'example/web',
                        'networks': {
                            'foobar': {'aliases': ['foo', 'bar']}
                        }
                    }
                },
                'networks': {'foobar': {}, 'baz': {}}
            }
        )

        override_file = config.ConfigFile(
            'override.yaml',
            {
                'version': '2',
                'services': {
                    'web': {
                        'networks': ['baz']
                    }
                }
            }
        )

        details = config.ConfigDetails('.', [base_file, override_file])
        web_service = config.load(details).services[0]
        assert web_service['networks'] == {
            'foobar': {'aliases': ['bar', 'foo']},
            'baz': {}
        }

    def test_load_with_multiple_files_mismatched_networks_format_inverse_order(self):
        base_file = config.ConfigFile(
            'override.yaml',
            {
                'version': '2',
                'services': {
                    'web': {
                        'networks': ['baz']
                    }
                }
            }
        )
        override_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '2',
                'services': {
                    'web': {
                        'image': 'example/web',
                        'networks': {
                            'foobar': {'aliases': ['foo', 'bar']}
                        }
                    }
                },
                'networks': {'foobar': {}, 'baz': {}}
            }
        )

        details = config.ConfigDetails('.', [base_file, override_file])
        web_service = config.load(details).services[0]
        assert web_service['networks'] == {
            'foobar': {'aliases': ['bar', 'foo']},
            'baz': {}
        }

    def test_load_with_multiple_files_v2(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '2',
                'services': {
                    'web': {
                        'image': 'example/web',
                        'depends_on': ['db'],
                    },
                    'db': {
                        'image': 'example/db',
                    }
                },
            })
        override_file = config.ConfigFile(
            'override.yaml',
            {
                'version': '2',
                'services': {
                    'web': {
                        'build': '/',
                        'volumes': ['/home/user/project:/code'],
                        'depends_on': ['other'],
                    },
                    'other': {
                        'image': 'example/other',
                    }
                }
            })
        details = config.ConfigDetails('.', [base_file, override_file])

        service_dicts = config.load(details).services
        expected = [
            {
                'name': 'web',
                'build': {'context': os.path.abspath('/')},
                'image': 'example/web',
                'volumes': [VolumeSpec.parse('/home/user/project:/code')],
                'depends_on': {
                    'db': {'condition': 'service_started'},
                    'other': {'condition': 'service_started'},
                },
            },
            {
                'name': 'db',
                'image': 'example/db',
            },
            {
                'name': 'other',
                'image': 'example/other',
            },
        ]
        assert service_sort(service_dicts) == service_sort(expected)

    @mock.patch.dict(os.environ)
    def test_load_with_multiple_files_v3_2(self):
        os.environ['COMPOSE_CONVERT_WINDOWS_PATHS'] = 'true'
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '3.2',
                'services': {
                    'web': {
                        'image': 'example/web',
                        'volumes': [
                            {'source': '/a', 'target': '/b', 'type': 'bind'},
                            {'source': 'vol', 'target': '/x', 'type': 'volume', 'read_only': True}
                        ],
                        'stop_grace_period': '30s',
                    }
                },
                'volumes': {'vol': {}}
            }
        )

        override_file = config.ConfigFile(
            'override.yaml',
            {
                'version': '3.2',
                'services': {
                    'web': {
                        'volumes': ['/c:/b', '/anonymous']
                    }
                }
            }
        )
        details = config.ConfigDetails('.', [base_file, override_file])
        service_dicts = config.load(details).services
        svc_volumes = map(lambda v: v.repr(), service_dicts[0]['volumes'])
        for vol in svc_volumes:
            assert vol in [
                '/anonymous',
                '/c:/b:rw',
                {'source': 'vol', 'target': '/x', 'type': 'volume', 'read_only': True}
            ]
        assert service_dicts[0]['stop_grace_period'] == '30s'

    @mock.patch.dict(os.environ)
    def test_volume_mode_override(self):
        os.environ['COMPOSE_CONVERT_WINDOWS_PATHS'] = 'true'
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '2.3',
                'services': {
                    'web': {
                        'image': 'example/web',
                        'volumes': ['/c:/b:rw']
                    }
                },
            }
        )

        override_file = config.ConfigFile(
            'override.yaml',
            {
                'version': '2.3',
                'services': {
                    'web': {
                        'volumes': ['/c:/b:ro']
                    }
                }
            }
        )
        details = config.ConfigDetails('.', [base_file, override_file])
        service_dicts = config.load(details).services
        svc_volumes = list(map(lambda v: v.repr(), service_dicts[0]['volumes']))
        assert svc_volumes == ['/c:/b:ro']

    def test_undeclared_volume_v2(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '2',
                'services': {
                    'web': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'volumes': ['data0028:/data:ro'],
                    },
                },
            }
        )
        details = config.ConfigDetails('.', [base_file])
        with pytest.raises(ConfigurationError):
            config.load(details)

        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '2',
                'services': {
                    'web': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'volumes': ['./data0028:/data:ro'],
                    },
                },
            }
        )
        details = config.ConfigDetails('.', [base_file])
        config_data = config.load(details)
        volume = config_data.services[0].get('volumes')[0]
        assert not volume.is_named_volume

    def test_undeclared_volume_v1(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'web': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'volumes': ['data0028:/data:ro'],
                },
            }
        )
        details = config.ConfigDetails('.', [base_file])
        config_data = config.load(details)
        volume = config_data.services[0].get('volumes')[0]
        assert volume.external == 'data0028'
        assert volume.is_named_volume

    def test_volumes_long_syntax(self):
        base_file = config.ConfigFile(
            'base.yaml', {
                'version': '2.3',
                'services': {
                    'web': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'volumes': [
                            {
                                'target': '/anonymous', 'type': 'volume'
                            }, {
                                'source': '/abc', 'target': '/xyz', 'type': 'bind'
                            }, {
                                'source': '\\\\.\\pipe\\abcd', 'target': '/named_pipe', 'type': 'npipe'
                            }, {
                                'type': 'tmpfs', 'target': '/tmpfs'
                            }
                        ]
                    },
                },
            },
        )
        details = config.ConfigDetails('.', [base_file])
        config_data = config.load(details)
        volumes = config_data.services[0].get('volumes')
        anon_volume = [v for v in volumes if v.target == '/anonymous'][0]
        tmpfs_mount = [v for v in volumes if v.type == 'tmpfs'][0]
        host_mount = [v for v in volumes if v.type == 'bind'][0]
        npipe_mount = [v for v in volumes if v.type == 'npipe'][0]

        assert anon_volume.type == 'volume'
        assert not anon_volume.is_named_volume

        assert tmpfs_mount.target == '/tmpfs'
        assert not tmpfs_mount.is_named_volume

        assert host_mount.source == '/abc'
        assert host_mount.target == '/xyz'
        assert not host_mount.is_named_volume

        assert npipe_mount.source == '\\\\.\\pipe\\abcd'
        assert npipe_mount.target == '/named_pipe'
        assert not npipe_mount.is_named_volume

    def test_load_bind_mount_relative_path(self):
        expected_source = 'C:\\tmp\\web' if IS_WINDOWS_PLATFORM else '/tmp/web'
        base_file = config.ConfigFile(
            'base.yaml', {
                'version': '3.4',
                'services': {
                    'web': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'volumes': [
                            {'type': 'bind', 'source': './web', 'target': '/web'},
                        ],
                    },
                },
            },
        )

        details = config.ConfigDetails('/tmp', [base_file])
        config_data = config.load(details)
        mount = config_data.services[0].get('volumes')[0]
        assert mount.target == '/web'
        assert mount.type == 'bind'
        assert mount.source == expected_source

    def test_load_bind_mount_relative_path_with_tilde(self):
        base_file = config.ConfigFile(
            'base.yaml', {
                'version': '3.4',
                'services': {
                    'web': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'volumes': [
                            {'type': 'bind', 'source': '~/web', 'target': '/web'},
                        ],
                    },
                },
            },
        )

        details = config.ConfigDetails('.', [base_file])
        config_data = config.load(details)
        mount = config_data.services[0].get('volumes')[0]
        assert mount.target == '/web'
        assert mount.type == 'bind'
        assert (
            not mount.source.startswith('~') and mount.source.endswith(
                '{}web'.format(os.path.sep)
            )
        )

    def test_config_invalid_ipam_config(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': str(VERSION),
                        'networks': {
                            'foo': {
                                'driver': 'default',
                                'ipam': {
                                    'driver': 'default',
                                    'config': ['172.18.0.0/16'],
                                }
                            }
                        }
                    },
                    filename='filename.yml',
                )
            )
        assert ('networks.foo.ipam.config contains an invalid type,'
                ' it should be an object') in excinfo.exconly()

    def test_config_valid_ipam_config(self):
        ipam_config = {
            'subnet': '172.28.0.0/16',
            'ip_range': '172.28.5.0/24',
            'gateway': '172.28.5.254',
            'aux_addresses': {
                'host1': '172.28.1.5',
                'host2': '172.28.1.6',
                'host3': '172.28.1.7',
            },
        }
        networks = config.load(
            build_config_details(
                {
                    'networks': {
                        'foo': {
                            'driver': 'default',
                            'ipam': {
                                'driver': 'default',
                                'config': [ipam_config],
                            }
                        }
                    }
                },
                filename='filename.yml',
            )
        ).networks

        assert 'foo' in networks
        assert networks['foo']['ipam']['config'] == [ipam_config]

    def test_config_valid_service_names(self):
        for valid_name in ['_', '-', '.__.', '_what-up.', 'what_.up----', 'whatup']:
            services = config.load(
                build_config_details(
                    {valid_name: {'image': 'busybox'}},
                    'tests/fixtures/extends',
                    'common.yml')).services
            assert services[0]['name'] == valid_name

    def test_config_hint(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': str(VERSION),
                        'services': {
                            'foo': {'image': 'busybox', 'privilige': 'something'},
                        }
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "(did you mean 'privileged'?)" in excinfo.exconly()

    def test_load_errors_on_uppercase_with_no_image(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details({
                'Foo': {'build': '.'},
            }, 'tests/fixtures/build-ctx'))
            assert "Service 'Foo' contains uppercase characters" in exc.exconly()

    def test_invalid_config_v1(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': str(VERSION),
                        'services': {
                            'foo': {'image': 1},
                        }
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "foo.image contains an invalid type, it should be a string" \
            in excinfo.exconly()

    def test_invalid_config_v2(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': '2',
                        'services': {
                            'foo': {'image': 1},
                        },
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "services.foo.image contains an invalid type, it should be a string" \
            in excinfo.exconly()

    def test_invalid_config_build_and_image_specified_v1(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'foo': {'image': 'busybox', 'build': '.'},
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "foo has both an image and build path specified." in excinfo.exconly()

    def test_invalid_config_type_should_be_an_array(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': str(VERSION),
                        'services': {
                            'foo': {'image': 'busybox', 'links': 'an_link'},
                        }
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "foo.links contains an invalid type, it should be an array" \
            in excinfo.exconly()

    def test_invalid_config_not_a_dictionary(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    ['foo', 'lol'],
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "Top level object in 'filename.yml' needs to be an object" \
            in excinfo.exconly()

    def test_invalid_config_not_unique_items(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': str(VERSION),
                        'services': {
                            'web': {'build': '.', 'devices': ['/dev/foo:/dev/foo', '/dev/foo:/dev/foo']}
                        }
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "has non-unique elements" in excinfo.exconly()

    def test_invalid_list_of_strings_format(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': str(VERSION),
                        'services': {
                            'web': {'build': '.', 'command': [1]}
                        }
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "web.command contains 1, which is an invalid type, it should be a string" \
            in excinfo.exconly()

    def test_load_config_dockerfile_without_build_raises_error_v1(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details({
                'web': {
                    'image': 'busybox',
                    'dockerfile': 'Dockerfile.alt'
                }
            }))

        assert "web has both an image and alternate Dockerfile." in exc.exconly()

    def test_config_extra_hosts_string_raises_validation_error(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': str(VERSION),
                        'services': {
                            'web': {
                                'image': 'busybox',
                                'extra_hosts': 'somehost:162.242.195.82'}}
                    },
                    'working_dir',
                    'filename.yml'
                )
            )

        assert "web.extra_hosts contains an invalid type" \
            in excinfo.exconly()

    def test_config_extra_hosts_list_of_dicts_validation_error(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': str(VERSION),
                        'services': {
                            'web': {
                                'image': 'busybox',
                                'extra_hosts': [
                                    {'somehost': '162.242.195.82'},
                                    {'otherhost': '50.31.209.229'}
                                ]}}
                    },
                    'working_dir',
                    'filename.yml'
                )
            )

        assert "web.extra_hosts contains {\"somehost\": \"162.242.195.82\"}, " \
               "which is an invalid type, it should be a string" \
            in excinfo.exconly()

    def test_config_ulimits_invalid_keys_validation_error(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details(
                {
                    'version': str(VERSION),
                    'services': {
                        'web': {
                            'image': 'busybox',
                            'ulimits': {
                                'nofile': {
                                    "not_soft_or_hard": 100,
                                    "soft": 10000,
                                    "hard": 20000,
                                }
                            }
                        }
                    }
                },
                'working_dir',
                'filename.yml'))

        assert "web.ulimits.nofile contains unsupported option: 'not_soft_or_hard'" \
            in exc.exconly()

    def test_config_ulimits_required_keys_validation_error(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details(
                {
                    'version': str(VERSION),
                    'services': {
                        'web': {
                            'image': 'busybox',
                            'ulimits': {'nofile': {"soft": 10000}}
                        }
                    }
                },
                'working_dir',
                'filename.yml'))
        assert "web.ulimits.nofile" in exc.exconly()
        assert "'hard' is a required property" in exc.exconly()

    def test_config_ulimits_soft_greater_than_hard_error(self):
        expected = "'soft' value can not be greater than 'hard' value"

        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details(
                {
                    'version': str(VERSION),
                    'services': {
                        'web': {
                            'image': 'busybox',
                            'ulimits': {
                                'nofile': {"soft": 10000, "hard": 1000}
                            }
                        }
                    }
                },
                'working_dir',
                'filename.yml'))
        assert expected in exc.exconly()

    def test_valid_config_which_allows_two_type_definitions(self):
        expose_values = [["8000"], [8000]]
        for expose in expose_values:
            service = config.load(
                build_config_details(
                    {
                        'version': str(VERSION),
                        'services': {
                            'web': {
                                'image': 'busybox',
                                'expose': expose}}},
                    'working_dir',
                    'filename.yml'
                )
            ).services
            assert service[0]['expose'] == expose

    def test_valid_config_oneof_string_or_list(self):
        entrypoint_values = [["sh"], "sh"]
        for entrypoint in entrypoint_values:
            service = config.load(
                build_config_details(
                    {
                        'version': str(VERSION),
                        'services': {
                            'web': {
                                'image': 'busybox',
                                'entrypoint': entrypoint}}},
                    'working_dir',
                    'filename.yml'
                )
            ).services
            assert service[0]['entrypoint'] == entrypoint

    def test_logs_warning_for_boolean_in_environment(self):
        config_details = build_config_details({
            'version': str(VERSION),
            'services': {
                'web': {
                    'image': 'busybox',
                    'environment': {'SHOW_STUFF': True}
                }
            }
        })

        with pytest.raises(ConfigurationError) as exc:
            config.load(config_details)

        assert "contains true, which is an invalid type" in exc.exconly()

    def test_config_valid_environment_dict_key_contains_dashes(self):
        services = config.load(
            build_config_details(
                {
                    'version': str(VERSION),
                    'services': {
                        'web': {
                            'image': 'busybox',
                            'environment': {'SPRING_JPA_HIBERNATE_DDL-AUTO': 'none'}}}},
                'working_dir',
                'filename.yml'
            )
        ).services
        assert services[0]['environment']['SPRING_JPA_HIBERNATE_DDL-AUTO'] == 'none'

    def test_load_yaml_with_yaml_error(self):
        tmpdir = tempfile.mkdtemp('invalid_yaml_test')
        self.addCleanup(shutil.rmtree, tmpdir)
        invalid_yaml_file = os.path.join(tmpdir, 'docker-compose.yml')
        with open(invalid_yaml_file, mode="w") as invalid_yaml_file_fh:
            invalid_yaml_file_fh.write("""
web:
    this is bogus: ok: what
            """)
        with pytest.raises(ConfigurationError) as exc:
            config.load_yaml(str(invalid_yaml_file))

        assert 'line 3, column 22' in exc.exconly()

    def test_load_yaml_with_bom(self):
        tmpdir = tempfile.mkdtemp('bom_yaml')
        self.addCleanup(shutil.rmtree, tmpdir)
        bom_yaml = os.path.join(tmpdir, 'docker-compose.yml')
        with codecs.open(str(bom_yaml), 'w', encoding='utf-8') as f:
            f.write('''\ufeff
                version: '2.3'
                volumes:
                    park_bom:
            ''')
        assert config.load_yaml(str(bom_yaml)) == {
            'version': '2.3',
            'volumes': {'park_bom': None}
        }

    def test_validate_extra_hosts_invalid(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details({
                'version': str(VERSION),
                'services': {
                    'web': {
                        'image': 'alpine',
                        'extra_hosts': "www.example.com: 192.168.0.17",
                    }
                }
            }))
        assert "web.extra_hosts contains an invalid type" in exc.exconly()

    def test_validate_extra_hosts_invalid_list(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details({
                'version': str(VERSION),
                'services': {
                    'web': {
                        'image': 'alpine',
                        'extra_hosts': [
                            {'www.example.com': '192.168.0.17'},
                            {'api.example.com': '192.168.0.18'}
                        ],
                    }
                }
            }))
        assert "which is an invalid type" in exc.exconly()

    def test_normalize_dns_options(self):
        actual = config.load(build_config_details({
            'version': str(VERSION),
            'services': {
                'web': {
                    'image': 'alpine',
                    'dns': '8.8.8.8',
                    'dns_search': 'domain.local',
                }
            }
        }))
        assert actual.services == [
            {
                'name': 'web',
                'image': 'alpine',
                'dns': ['8.8.8.8'],
                'dns_search': ['domain.local'],
            }
        ]

    def test_tmpfs_option(self):
        actual = config.load(build_config_details({
            'version': '2',
            'services': {
                'web': {
                    'image': 'alpine',
                    'tmpfs': '/run',
                }
            }
        }))
        assert actual.services == [
            {
                'name': 'web',
                'image': 'alpine',
                'tmpfs': ['/run'],
            }
        ]

    def test_oom_score_adj_option(self):

        actual = config.load(build_config_details({
            'version': '2',
            'services': {
                'web': {
                    'image': 'alpine',
                    'oom_score_adj': 500
                }
            }
        }))

        assert actual.services == [
            {
                'name': 'web',
                'image': 'alpine',
                'oom_score_adj': 500
            }
        ]

    def test_swappiness_option(self):
        actual = config.load(build_config_details({
            'version': '2',
            'services': {
                'web': {
                    'image': 'alpine',
                    'mem_swappiness': 10,
                }
            }
        }))
        assert actual.services == [
            {
                'name': 'web',
                'image': 'alpine',
                'mem_swappiness': 10,
            }
        ]

    @data(
        '2 ',
        '3.',
        '3.0.0',
        '3.0.a',
        '3.a',
        '3a')
    def test_invalid_version_formats(self, version):
        content = {
            'version': version,
            'services': {
                'web': {
                    'image': 'alpine',
                }
            }
        }
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details(content))
        assert 'Version "{}" in "filename.yml" is invalid.'.format(version) in exc.exconly()

    def test_group_add_option(self):
        actual = config.load(build_config_details({
            'version': '2',
            'services': {
                'web': {
                    'image': 'alpine',
                    'group_add': ["docker", 777]
                }
            }
        }))

        assert actual.services == [
            {
                'name': 'web',
                'image': 'alpine',
                'group_add': ["docker", 777]
            }
        ]

    def test_dns_opt_option(self):
        actual = config.load(build_config_details({
            'version': '2',
            'services': {
                'web': {
                    'image': 'alpine',
                    'dns_opt': ["use-vc", "no-tld-query"]
                }
            }
        }))

        assert actual.services == [
            {
                'name': 'web',
                'image': 'alpine',
                'dns_opt': ["use-vc", "no-tld-query"]
            }
        ]

    def test_isolation_option(self):
        actual = config.load(build_config_details({
            'services': {
                'web': {
                    'image': 'win10',
                    'isolation': 'hyperv'
                }
            }
        }))

        assert actual.services == [
            {
                'name': 'web',
                'image': 'win10',
                'isolation': 'hyperv',
            }
        ]

    def test_runtime_option(self):
        actual = config.load(build_config_details({
            'services': {
                'web': {
                    'image': 'nvidia/cuda',
                    'runtime': 'nvidia'
                }
            }
        }))

        assert actual.services == [
            {
                'name': 'web',
                'image': 'nvidia/cuda',
                'runtime': 'nvidia',
            }
        ]

    def test_merge_service_dicts_from_files_with_extends_in_base(self):
        base = {
            'volumes': ['.:/app'],
            'extends': {'service': 'app'}
        }
        override = {
            'image': 'alpine:edge',
        }
        actual = config.merge_service_dicts_from_files(
            base,
            override,
            DEFAULT_VERSION)
        assert actual == {
            'image': 'alpine:edge',
            'volumes': ['.:/app'],
            'extends': {'service': 'app'}
        }

    def test_merge_service_dicts_from_files_with_extends_in_override(self):
        base = {
            'volumes': ['.:/app'],
            'extends': {'service': 'app'}
        }
        override = {
            'image': 'alpine:edge',
            'extends': {'service': 'foo'}
        }
        actual = config.merge_service_dicts_from_files(
            base,
            override,
            DEFAULT_VERSION)
        assert actual == {
            'image': 'alpine:edge',
            'volumes': ['.:/app'],
            'extends': {'service': 'foo'}
        }

    def test_merge_service_dicts_heterogeneous(self):
        base = {
            'volumes': ['.:/app'],
            'ports': ['5432']
        }
        override = {
            'image': 'alpine:edge',
            'ports': [5432]
        }
        actual = config.merge_service_dicts_from_files(
            base,
            override,
            DEFAULT_VERSION)
        assert actual == {
            'image': 'alpine:edge',
            'volumes': ['.:/app'],
            'ports': types.ServicePort.parse('5432')
        }

    def test_merge_service_dicts_heterogeneous_2(self):
        base = {
            'volumes': ['.:/app'],
            'ports': [5432]
        }
        override = {
            'image': 'alpine:edge',
            'ports': ['5432']
        }
        actual = config.merge_service_dicts_from_files(
            base,
            override,
            DEFAULT_VERSION)
        assert actual == {
            'image': 'alpine:edge',
            'volumes': ['.:/app'],
            'ports': types.ServicePort.parse('5432')
        }

    def test_merge_service_dicts_ports_sorting(self):
        base = {
            'ports': [5432]
        }
        override = {
            'image': 'alpine:edge',
            'ports': ['5432/udp']
        }
        actual = config.merge_service_dicts_from_files(
            base,
            override,
            DEFAULT_VERSION)
        assert len(actual['ports']) == 2
        assert types.ServicePort.parse('5432')[0] in actual['ports']
        assert types.ServicePort.parse('5432/udp')[0] in actual['ports']

    def test_merge_service_dicts_heterogeneous_volumes(self):
        base = {
            'volumes': ['/a:/b', '/x:/z'],
        }

        override = {
            'image': 'alpine:edge',
            'volumes': [
                {'source': '/e', 'target': '/b', 'type': 'bind'},
                {'source': '/c', 'target': '/d', 'type': 'bind'}
            ]
        }

        actual = config.merge_service_dicts_from_files(
            base, override, VERSION
        )

        assert actual['volumes'] == [
            {'source': '/e', 'target': '/b', 'type': 'bind'},
            {'source': '/c', 'target': '/d', 'type': 'bind'},
            '/x:/z'
        ]

    def test_merge_logging_v1(self):
        base = {
            'image': 'alpine:edge',
            'log_driver': 'something',
            'log_opt': {'foo': 'three'},
        }
        override = {
            'image': 'alpine:edge',
            'command': 'true',
        }
        actual = config.merge_service_dicts(base, override, V1)
        assert actual == {
            'image': 'alpine:edge',
            'log_driver': 'something',
            'log_opt': {'foo': 'three'},
            'command': 'true',
        }

    def test_merge_logging_v2(self):
        base = {
            'image': 'alpine:edge',
            'logging': {
                'driver': 'json-file',
                'options': {
                    'frequency': '2000',
                    'timeout': '23'
                }
            }
        }
        override = {
            'logging': {
                'options': {
                    'timeout': '360',
                    'pretty-print': 'on'
                }
            }
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': 'alpine:edge',
            'logging': {
                'driver': 'json-file',
                'options': {
                    'frequency': '2000',
                    'timeout': '360',
                    'pretty-print': 'on'
                }
            }
        }

    def test_merge_logging_v2_override_driver(self):
        base = {
            'image': 'alpine:edge',
            'logging': {
                'driver': 'json-file',
                'options': {
                    'frequency': '2000',
                    'timeout': '23'
                }
            }
        }
        override = {
            'logging': {
                'driver': 'syslog',
                'options': {
                    'timeout': '360',
                    'pretty-print': 'on'
                }
            }
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': 'alpine:edge',
            'logging': {
                'driver': 'syslog',
                'options': {
                    'timeout': '360',
                    'pretty-print': 'on'
                }
            }
        }

    def test_merge_logging_v2_no_base_driver(self):
        base = {
            'image': 'alpine:edge',
            'logging': {
                'options': {
                    'frequency': '2000',
                    'timeout': '23'
                }
            }
        }
        override = {
            'logging': {
                'driver': 'json-file',
                'options': {
                    'timeout': '360',
                    'pretty-print': 'on'
                }
            }
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': 'alpine:edge',
            'logging': {
                'driver': 'json-file',
                'options': {
                    'frequency': '2000',
                    'timeout': '360',
                    'pretty-print': 'on'
                }
            }
        }

    def test_merge_logging_v2_no_drivers(self):
        base = {
            'image': 'alpine:edge',
            'logging': {
                'options': {
                    'frequency': '2000',
                    'timeout': '23'
                }
            }
        }
        override = {
            'logging': {
                'options': {
                    'timeout': '360',
                    'pretty-print': 'on'
                }
            }
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': 'alpine:edge',
            'logging': {
                'options': {
                    'frequency': '2000',
                    'timeout': '360',
                    'pretty-print': 'on'
                }
            }
        }

    def test_merge_logging_v2_no_override_options(self):
        base = {
            'image': 'alpine:edge',
            'logging': {
                'driver': 'json-file',
                'options': {
                    'frequency': '2000',
                    'timeout': '23'
                }
            }
        }
        override = {
            'logging': {
                'driver': 'syslog'
            }
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': 'alpine:edge',
            'logging': {
                'driver': 'syslog',
            }
        }

    def test_merge_logging_v2_no_base(self):
        base = {
            'image': 'alpine:edge'
        }
        override = {
            'logging': {
                'driver': 'json-file',
                'options': {
                    'frequency': '2000'
                }
            }
        }
        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': 'alpine:edge',
            'logging': {
                'driver': 'json-file',
                'options': {
                    'frequency': '2000'
                }
            }
        }

    def test_merge_logging_v2_no_override(self):
        base = {
            'image': 'alpine:edge',
            'logging': {
                'driver': 'syslog',
                'options': {
                    'frequency': '2000'
                }
            }
        }
        override = {}
        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': 'alpine:edge',
            'logging': {
                'driver': 'syslog',
                'options': {
                    'frequency': '2000'
                }
            }
        }

    def test_merge_mixed_ports(self):
        base = {
            'image': BUSYBOX_IMAGE_WITH_TAG,
            'command': 'top',
            'ports': [
                {
                    'target': '1245',
                    'published': '1245',
                    'protocol': 'udp',
                }
            ]
        }

        override = {
            'ports': ['1245:1245/udp']
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': BUSYBOX_IMAGE_WITH_TAG,
            'command': 'top',
            'ports': [types.ServicePort('1245', '1245', 'udp', None, None)]
        }

    def test_merge_depends_on_no_override(self):
        base = {
            'image': 'busybox',
            'depends_on': {
                'app1': {'condition': 'service_started'},
                'app2': {'condition': 'service_healthy'},
                'app3': {'condition': 'service_completed_successfully'}
            }
        }
        override = {}
        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == base

    def test_merge_depends_on_mixed_syntax(self):
        base = {
            'image': 'busybox',
            'depends_on': {
                'app1': {'condition': 'service_started'},
                'app2': {'condition': 'service_healthy'},
                'app3': {'condition': 'service_completed_successfully'}
            }
        }
        override = {
            'depends_on': ['app4']
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': 'busybox',
            'depends_on': {
                'app1': {'condition': 'service_started'},
                'app2': {'condition': 'service_healthy'},
                'app3': {'condition': 'service_completed_successfully'},
                'app4': {'condition': 'service_started'},
            }
        }

    def test_empty_environment_key_allowed(self):
        service_dict = config.load(
            build_config_details(
                {
                    'web': {
                        'build': '.',
                        'environment': {
                            'POSTGRES_PASSWORD': ''
                        },
                    },
                },
                '.',
                None,
            )
        ).services[0]
        assert service_dict['environment']['POSTGRES_PASSWORD'] == ''

    def test_merge_pid(self):
        # Regression: https://github.com/docker/compose/issues/4184
        base = {
            'image': 'busybox',
            'pid': 'host'
        }

        override = {
            'labels': {'com.docker.compose.test': 'yes'}
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': 'busybox',
            'pid': 'host',
            'labels': {'com.docker.compose.test': 'yes'}
        }

    def test_merge_different_secrets(self):
        base = {
            'image': 'busybox',
            'secrets': [
                {'source': 'src.txt'}
            ]
        }
        override = {'secrets': ['other-src.txt']}

        actual = config.merge_service_dicts(base, override, VERSION)
        assert secret_sort(actual['secrets']) == secret_sort([
            {'source': 'src.txt'},
            {'source': 'other-src.txt'}
        ])

    def test_merge_secrets_override(self):
        base = {
            'image': 'busybox',
            'secrets': ['src.txt'],
        }
        override = {
            'secrets': [
                {
                    'source': 'src.txt',
                    'target': 'data.txt',
                    'mode': 0o400
                }
            ]
        }
        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual['secrets'] == override['secrets']

    def test_merge_different_configs(self):
        base = {
            'image': 'busybox',
            'configs': [
                {'source': 'src.txt'}
            ]
        }
        override = {'configs': ['other-src.txt']}

        actual = config.merge_service_dicts(base, override, VERSION)
        assert secret_sort(actual['configs']) == secret_sort([
            {'source': 'src.txt'},
            {'source': 'other-src.txt'}
        ])

    def test_merge_configs_override(self):
        base = {
            'image': 'busybox',
            'configs': ['src.txt'],
        }
        override = {
            'configs': [
                {
                    'source': 'src.txt',
                    'target': 'data.txt',
                    'mode': 0o400
                }
            ]
        }
        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual['configs'] == override['configs']

    def test_merge_deploy(self):
        base = {
            'image': 'busybox',
        }
        override = {
            'deploy': {
                'mode': 'global',
                'restart_policy': {
                    'condition': 'on-failure'
                }
            }
        }
        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual['deploy'] == override['deploy']

    def test_merge_deploy_override(self):
        base = {
            'deploy': {
                'endpoint_mode': 'vip',
                'labels': ['com.docker.compose.a=1', 'com.docker.compose.b=2'],
                'mode': 'replicated',
                'placement': {
                    'max_replicas_per_node': 1,
                    'constraints': [
                        'node.role == manager', 'engine.labels.aws == true'
                    ],
                    'preferences': [
                        {'spread': 'node.labels.zone'}, {'spread': 'x.d.z'}
                    ]
                },
                'replicas': 3,
                'resources': {
                    'limits': {'cpus': '0.50', 'memory': '50m'},
                    'reservations': {
                        'cpus': '0.1',
                        'generic_resources': [
                            {'discrete_resource_spec': {'kind': 'abc', 'value': 123}}
                        ],
                        'memory': '15m'
                    }
                },
                'restart_policy': {'condition': 'any', 'delay': '10s'},
                'update_config': {'delay': '10s', 'max_failure_ratio': 0.3}
            },
            'image': 'hello-world'
        }
        override = {
            'deploy': {
                'labels': {
                    'com.docker.compose.b': '21', 'com.docker.compose.c': '3'
                },
                'placement': {
                    'constraints': ['node.role == worker', 'engine.labels.dev == true'],
                    'preferences': [{'spread': 'node.labels.zone'}, {'spread': 'x.d.s'}]
                },
                'resources': {
                    'limits': {'memory': '200m'},
                    'reservations': {
                        'cpus': '0.78',
                        'generic_resources': [
                            {'discrete_resource_spec': {'kind': 'abc', 'value': 134}},
                            {'discrete_resource_spec': {'kind': 'xyz', 'value': 0.1}}
                        ]
                    }
                },
                'restart_policy': {'condition': 'on-failure', 'max_attempts': 42},
                'update_config': {'max_failure_ratio': 0.712, 'parallelism': 4}
            }
        }
        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual['deploy'] == {
            'mode': 'replicated',
            'endpoint_mode': 'vip',
            'labels': {
                'com.docker.compose.a': '1',
                'com.docker.compose.b': '21',
                'com.docker.compose.c': '3'
            },
            'placement': {
                'max_replicas_per_node': 1,
                'constraints': [
                    'engine.labels.aws == true', 'engine.labels.dev == true',
                    'node.role == manager', 'node.role == worker'
                ],
                'preferences': [
                    {'spread': 'node.labels.zone'}, {'spread': 'x.d.s'}, {'spread': 'x.d.z'}
                ]
            },
            'replicas': 3,
            'resources': {
                'limits': {'cpus': '0.50', 'memory': '200m'},
                'reservations': {
                    'cpus': '0.78',
                    'memory': '15m',
                    'generic_resources': [
                        {'discrete_resource_spec': {'kind': 'abc', 'value': 134}},
                        {'discrete_resource_spec': {'kind': 'xyz', 'value': 0.1}},
                    ]
                }
            },
            'restart_policy': {
                'condition': 'on-failure',
                'delay': '10s',
                'max_attempts': 42,
            },
            'update_config': {
                'max_failure_ratio': 0.712,
                'delay': '10s',
                'parallelism': 4
            }
        }

    def test_merge_credential_spec(self):
        base = {
            'image': 'bb',
            'credential_spec': {
                'file': '/hello-world',
            }
        }

        override = {
            'credential_spec': {
                'registry': 'revolution.com',
            }
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual['credential_spec'] == override['credential_spec']

    def test_merge_scale(self):
        base = {
            'image': 'bar',
            'scale': 2,
        }

        override = {
            'scale': 4,
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {'image': 'bar', 'scale': 4}

    def test_merge_blkio_config(self):
        base = {
            'image': 'bar',
            'blkio_config': {
                'weight': 300,
                'weight_device': [
                    {'path': '/dev/sda1', 'weight': 200}
                ],
                'device_read_iops': [
                    {'path': '/dev/sda1', 'rate': 300}
                ],
                'device_write_iops': [
                    {'path': '/dev/sda1', 'rate': 1000}
                ]
            }
        }

        override = {
            'blkio_config': {
                'weight': 450,
                'weight_device': [
                    {'path': '/dev/sda2', 'weight': 400}
                ],
                'device_read_iops': [
                    {'path': '/dev/sda1', 'rate': 2000}
                ],
                'device_read_bps': [
                    {'path': '/dev/sda1', 'rate': 1024}
                ]
            }
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': 'bar',
            'blkio_config': {
                'weight': override['blkio_config']['weight'],
                'weight_device': (
                    base['blkio_config']['weight_device'] +
                    override['blkio_config']['weight_device']
                ),
                'device_read_iops': override['blkio_config']['device_read_iops'],
                'device_read_bps': override['blkio_config']['device_read_bps'],
                'device_write_iops': base['blkio_config']['device_write_iops']
            }
        }

    def test_merge_extra_hosts(self):
        base = {
            'image': 'bar',
            'extra_hosts': {
                'foo': '1.2.3.4',
            }
        }

        override = {
            'extra_hosts': ['bar:5.6.7.8', 'foo:127.0.0.1']
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual['extra_hosts'] == {
            'foo': '127.0.0.1',
            'bar': '5.6.7.8',
        }

    def test_merge_healthcheck_config(self):
        base = {
            'image': 'bar',
            'healthcheck': {
                'start_period': 1000,
                'interval': 3000,
                'test': ['true']
            }
        }

        override = {
            'healthcheck': {
                'interval': 5000,
                'timeout': 10000,
                'test': ['echo', 'OK'],
            }
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual['healthcheck'] == {
            'start_period': base['healthcheck']['start_period'],
            'test': override['healthcheck']['test'],
            'interval': override['healthcheck']['interval'],
            'timeout': override['healthcheck']['timeout'],
        }

    def test_merge_healthcheck_override_disables(self):
        base = {
            'image': 'bar',
            'healthcheck': {
                'start_period': 1000,
                'interval': 3000,
                'timeout': 2000,
                'retries': 3,
                'test': ['true']
            }
        }

        override = {
            'healthcheck': {
                'disabled': True
            }
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual['healthcheck'] == {'disabled': True}

    def test_merge_healthcheck_override_enables(self):
        base = {
            'image': 'bar',
            'healthcheck': {
                'disabled': True
            }
        }

        override = {
            'healthcheck': {
                'disabled': False,
                'start_period': 1000,
                'interval': 3000,
                'timeout': 2000,
                'retries': 3,
                'test': ['true']
            }
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual['healthcheck'] == override['healthcheck']

    def test_merge_device_cgroup_rules(self):
        base = {
            'image': 'bar',
            'device_cgroup_rules': ['c 7:128 rwm', 'x 3:244 rw']
        }

        override = {
            'device_cgroup_rules': ['c 7:128 rwm', 'f 0:128 n']
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert sorted(actual['device_cgroup_rules']) == sorted(
            ['c 7:128 rwm', 'x 3:244 rw', 'f 0:128 n']
        )

    def test_merge_isolation(self):
        base = {
            'image': 'bar',
            'isolation': 'default',
        }

        override = {
            'isolation': 'hyperv',
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual == {
            'image': 'bar',
            'isolation': 'hyperv',
        }

    def test_merge_storage_opt(self):
        base = {
            'image': 'bar',
            'storage_opt': {
                'size': '1G',
                'readonly': 'false',
            }
        }

        override = {
            'storage_opt': {
                'size': '2G',
                'encryption': 'aes',
            }
        }

        actual = config.merge_service_dicts(base, override, VERSION)
        assert actual['storage_opt'] == {
            'size': '2G',
            'readonly': 'false',
            'encryption': 'aes',
        }

    def test_external_volume_config(self):
        config_details = build_config_details({
            'version': '2',
            'services': {
                'bogus': {'image': 'busybox'}
            },
            'volumes': {
                'ext': {'external': True},
                'ext2': {'external': {'name': 'aliased'}}
            }
        })
        config_result = config.load(config_details)
        volumes = config_result.volumes
        assert 'ext' in volumes
        assert volumes['ext']['external'] is True
        assert 'ext2' in volumes
        assert volumes['ext2']['external']['name'] == 'aliased'

    def test_external_volume_invalid_config(self):
        config_details = build_config_details({
            'version': '2',
            'services': {
                'bogus': {'image': 'busybox'}
            },
            'volumes': {
                'ext': {'external': True, 'driver': 'foo'}
            }
        })
        with pytest.raises(ConfigurationError):
            config.load(config_details)

    def test_depends_on_orders_services(self):
        config_details = build_config_details({
            'version': '2',
            'services': {
                'one': {'image': 'busybox', 'depends_on': ['three', 'two']},
                'two': {'image': 'busybox', 'depends_on': ['three']},
                'three': {'image': 'busybox'},
            },
        })
        actual = config.load(config_details)
        assert (
            [service['name'] for service in actual.services] ==
            ['three', 'two', 'one']
        )

    def test_depends_on_unknown_service_errors(self):
        config_details = build_config_details({
            'version': '2',
            'services': {
                'one': {'image': 'busybox', 'depends_on': ['three']},
            },
        })
        with pytest.raises(ConfigurationError) as exc:
            config.load(config_details)
        assert "Service 'one' depends on service 'three'" in exc.exconly()

    def test_linked_service_is_undefined(self):
        with pytest.raises(ConfigurationError):
            config.load(
                build_config_details({
                    'version': '2',
                    'services': {
                        'web': {'image': 'busybox', 'links': ['db:db']},
                    },
                })
            )

    def test_load_dockerfile_without_context(self):
        config_details = build_config_details({
            'version': '2',
            'services': {
                'one': {'build': {'dockerfile': 'Dockerfile.foo'}},
            },
        })
        with pytest.raises(ConfigurationError) as exc:
            config.load(config_details)
        assert 'has neither an image nor a build context' in exc.exconly()

    def test_load_secrets(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '3.1',
                'services': {
                    'web': {
                        'image': 'example/web',
                        'secrets': [
                            'one',
                            {
                                'source': 'source',
                                'target': 'target',
                                'uid': '100',
                                'gid': '200',
                                'mode': 0o777,
                            },
                        ],
                    },
                },
                'secrets': {
                    'one': {'file': 'secret.txt'},
                },
            })
        details = config.ConfigDetails('.', [base_file])
        service_dicts = config.load(details).services
        expected = [
            {
                'name': 'web',
                'image': 'example/web',
                'secrets': [
                    types.ServiceSecret('one', None, None, None, None, None),
                    types.ServiceSecret('source', 'target', '100', '200', 0o777, None),
                ],
            },
        ]
        assert service_sort(service_dicts) == service_sort(expected)

    def test_load_secrets_multi_file(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '3.1',
                'services': {
                    'web': {
                        'image': 'example/web',
                        'secrets': ['one'],
                    },
                },
                'secrets': {
                    'one': {'file': 'secret.txt'},
                },
            })
        override_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '3.1',
                'services': {
                    'web': {
                        'secrets': [
                            {
                                'source': 'source',
                                'target': 'target',
                                'uid': '100',
                                'gid': '200',
                                'mode': 0o777,
                            },
                        ],
                    },
                },
            })
        details = config.ConfigDetails('.', [base_file, override_file])
        service_dicts = config.load(details).services
        expected = [
            {
                'name': 'web',
                'image': 'example/web',
                'secrets': [
                    types.ServiceSecret('one', None, None, None, None, None),
                    types.ServiceSecret('source', 'target', '100', '200', 0o777, None),
                ],
            },
        ]
        assert service_sort(service_dicts) == service_sort(expected)

    def test_load_configs(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '3.3',
                'services': {
                    'web': {
                        'image': 'example/web',
                        'configs': [
                            'one',
                            {
                                'source': 'source',
                                'target': 'target',
                                'uid': '100',
                                'gid': '200',
                                'mode': 0o777,
                            },
                        ],
                    },
                },
                'configs': {
                    'one': {'file': 'secret.txt'},
                },
            })
        details = config.ConfigDetails('.', [base_file])
        service_dicts = config.load(details).services
        expected = [
            {
                'name': 'web',
                'image': 'example/web',
                'configs': [
                    types.ServiceConfig('one', None, None, None, None, None),
                    types.ServiceConfig('source', 'target', '100', '200', 0o777, None),
                ],
            },
        ]
        assert service_sort(service_dicts) == service_sort(expected)

    def test_load_configs_multi_file(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '3.3',
                'services': {
                    'web': {
                        'image': 'example/web',
                        'configs': ['one'],
                    },
                },
                'configs': {
                    'one': {'file': 'secret.txt'},
                },
            })
        override_file = config.ConfigFile(
            'base.yaml',
            {
                'version': '3.3',
                'services': {
                    'web': {
                        'configs': [
                            {
                                'source': 'source',
                                'target': 'target',
                                'uid': '100',
                                'gid': '200',
                                'mode': 0o777,
                            },
                        ],
                    },
                },
            })
        details = config.ConfigDetails('.', [base_file, override_file])
        service_dicts = config.load(details).services
        expected = [
            {
                'name': 'web',
                'image': 'example/web',
                'configs': [
                    types.ServiceConfig('one', None, None, None, None, None),
                    types.ServiceConfig('source', 'target', '100', '200', 0o777, None),
                ],
            },
        ]
        assert service_sort(service_dicts) == service_sort(expected)

    def test_config_convertible_label_types(self):
        config_details = build_config_details(
            {
                'version': '3.5',
                'services': {
                    'web': {
                        'build': {
                            'labels': {'testbuild': True},
                            'context': os.getcwd()
                        },
                        'labels': {
                            "key": 12345
                        }
                    },
                },
                'networks': {
                    'foo': {
                        'labels': {'network.ips.max': 1023}
                    }
                },
                'volumes': {
                    'foo': {
                        'labels': {'volume.is_readonly': False}
                    }
                },
                'secrets': {
                    'foo': {
                        'labels': {'secret.data.expires': 1546282120}
                    }
                },
                'configs': {
                    'foo': {
                        'labels': {'config.data.correction.value': -0.1412}
                    }
                }
            }
        )
        loaded_config = config.load(config_details)

        assert loaded_config.services[0]['build']['labels'] == {'testbuild': 'True'}
        assert loaded_config.services[0]['labels'] == {'key': '12345'}
        assert loaded_config.networks['foo']['labels']['network.ips.max'] == '1023'
        assert loaded_config.volumes['foo']['labels']['volume.is_readonly'] == 'False'
        assert loaded_config.secrets['foo']['labels']['secret.data.expires'] == '1546282120'
        assert loaded_config.configs['foo']['labels']['config.data.correction.value'] == '-0.1412'

    def test_config_invalid_label_types(self):
        config_details = build_config_details({
            'version': '2.3',
            'volumes': {
                'foo': {'labels': [1, 2, 3]}
            }
        })
        with pytest.raises(ConfigurationError):
            config.load(config_details)

    def test_service_volume_invalid_config(self):
        config_details = build_config_details(
            {
                'version': '3.2',
                'services': {
                    'web': {
                        'build': {
                            'context': '.',
                            'args': None,
                        },
                        'volumes': [
                            {
                                "type": "volume",
                                "source": "/data",
                                "garbage": {
                                    "and": "error"
                                }
                            }
                        ]
                    }
                }
            }
        )
        with pytest.raises(ConfigurationError) as exc:
            config.load(config_details)

        assert "services.web.volumes contains unsupported option: 'garbage'" in exc.exconly()

    def test_config_valid_service_label_validation(self):
        config_details = build_config_details(
            {
                'version': '3.5',
                'services': {
                    'web': {
                        'image': 'busybox',
                        'labels': {
                            "key": "string"
                        }
                    },
                },
            }
        )
        config.load(config_details)

    def test_config_duplicate_mount_points(self):
        config1 = build_config_details(
            {
                'version': '3.5',
                'services': {
                    'web': {
                        'image': 'busybox',
                        'volumes': ['/tmp/foo:/tmp/foo', '/tmp/foo:/tmp/foo:rw']
                    }
                }
            }
        )

        config2 = build_config_details(
            {
                'version': '3.5',
                'services': {
                    'web': {
                        'image': 'busybox',
                        'volumes': ['/x:/y', '/z:/y']
                    }
                }
            }
        )

        with self.assertRaises(ConfigurationError) as e:
            config.load(config1)
        self.assertEqual(str(e.exception), 'Duplicate mount points: [%s]' % (
            ', '.join(['/tmp/foo:/tmp/foo:rw']*2)))

        with self.assertRaises(ConfigurationError) as e:
            config.load(config2)
        self.assertEqual(str(e.exception), 'Duplicate mount points: [%s]' % (
            ', '.join(['/x:/y:rw', '/z:/y:rw'])))


class NetworkModeTest(unittest.TestCase):

    def test_network_mode_standard(self):
        config_data = config.load(build_config_details({
            'version': '2',
            'services': {
                'web': {
                    'image': 'busybox',
                    'command': "top",
                    'network_mode': 'bridge',
                },
            },
        }))

        assert config_data.services[0]['network_mode'] == 'bridge'

    def test_network_mode_standard_v1(self):
        config_data = config.load(build_config_details({
            'web': {
                'image': 'busybox',
                'command': "top",
                'net': 'bridge',
            },
        }))

        assert config_data.services[0]['network_mode'] == 'bridge'
        assert 'net' not in config_data.services[0]

    def test_network_mode_container(self):
        config_data = config.load(build_config_details({
            'version': '2',
            'services': {
                'web': {
                    'image': 'busybox',
                    'command': "top",
                    'network_mode': 'container:foo',
                },
            },
        }))

        assert config_data.services[0]['network_mode'] == 'container:foo'

    def test_network_mode_container_v1(self):
        config_data = config.load(build_config_details({
            'web': {
                'image': 'busybox',
                'command': "top",
                'net': 'container:foo',
            },
        }))

        assert config_data.services[0]['network_mode'] == 'container:foo'

    def test_network_mode_service(self):
        config_data = config.load(build_config_details({
            'version': '2',
            'services': {
                'web': {
                    'image': 'busybox',
                    'command': "top",
                    'network_mode': 'service:foo',
                },
                'foo': {
                    'image': 'busybox',
                    'command': "top",
                },
            },
        }))

        assert config_data.services[1]['network_mode'] == 'service:foo'

    def test_network_mode_service_v1(self):
        config_data = config.load(build_config_details({
            'web': {
                'image': 'busybox',
                'command': "top",
                'net': 'container:foo',
            },
            'foo': {
                'image': 'busybox',
                'command': "top",
            },
        }))

        assert config_data.services[1]['network_mode'] == 'service:foo'

    def test_network_mode_service_nonexistent(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(build_config_details({
                'version': '2',
                'services': {
                    'web': {
                        'image': 'busybox',
                        'command': "top",
                        'network_mode': 'service:foo',
                    },
                },
            }))

        assert "service 'foo' which is undefined" in excinfo.exconly()

    def test_network_mode_plus_networks_is_invalid(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(build_config_details({
                'version': '2',
                'services': {
                    'web': {
                        'image': 'busybox',
                        'command': "top",
                        'network_mode': 'bridge',
                        'networks': ['front'],
                    },
                },
                'networks': {
                    'front': None,
                }
            }))

        assert "'network_mode' and 'networks' cannot be combined" in excinfo.exconly()


class PortsTest(unittest.TestCase):
    INVALID_PORTS_TYPES = [
        {"1": "8000"},
        False,
        "8000",
        8000,
    ]

    NON_UNIQUE_SINGLE_PORTS = [
        ["8000", "8000"],
    ]

    INVALID_PORT_MAPPINGS = [
        ["8000-8004:8000-8002"],
        ["4242:4242-4244"],
    ]

    VALID_SINGLE_PORTS = [
        ["8000"],
        ["8000/tcp"],
        ["8000", "9000"],
        [8000],
        [8000, 9000],
    ]

    VALID_PORT_MAPPINGS = [
        ["8000:8050"],
        ["49153-49154:3002-3003"],
    ]

    def test_config_invalid_ports_type_validation(self):
        for invalid_ports in self.INVALID_PORTS_TYPES:
            with pytest.raises(ConfigurationError) as exc:
                self.check_config({'ports': invalid_ports})

            assert "contains an invalid type" in exc.value.msg

    def test_config_non_unique_ports_validation(self):
        for invalid_ports in self.NON_UNIQUE_SINGLE_PORTS:
            with pytest.raises(ConfigurationError) as exc:
                self.check_config({'ports': invalid_ports})

            assert "non-unique" in exc.value.msg

    @pytest.mark.skip(reason="Validator is one_off (generic error)")
    def test_config_invalid_ports_format_validation(self):
        for invalid_ports in self.INVALID_PORT_MAPPINGS:
            with pytest.raises(ConfigurationError) as exc:
                self.check_config({'ports': invalid_ports})

            assert "Port ranges don't match in length" in exc.value.msg

    def test_config_valid_ports_format_validation(self):
        for valid_ports in self.VALID_SINGLE_PORTS + self.VALID_PORT_MAPPINGS:
            self.check_config({'ports': valid_ports})

    def test_config_invalid_expose_type_validation(self):
        for invalid_expose in self.INVALID_PORTS_TYPES:
            with pytest.raises(ConfigurationError) as exc:
                self.check_config({'expose': invalid_expose})

            assert "contains an invalid type" in exc.value.msg

    def test_config_non_unique_expose_validation(self):
        for invalid_expose in self.NON_UNIQUE_SINGLE_PORTS:
            with pytest.raises(ConfigurationError) as exc:
                self.check_config({'expose': invalid_expose})

            assert "non-unique" in exc.value.msg

    def test_config_invalid_expose_format_validation(self):
        # Valid port mappings ARE NOT valid 'expose' entries
        for invalid_expose in self.INVALID_PORT_MAPPINGS + self.VALID_PORT_MAPPINGS:
            with pytest.raises(ConfigurationError) as exc:
                self.check_config({'expose': invalid_expose})

            assert "should be of the format" in exc.value.msg

    def test_config_valid_expose_format_validation(self):
        # Valid single ports ARE valid 'expose' entries
        for valid_expose in self.VALID_SINGLE_PORTS:
            self.check_config({'expose': valid_expose})

    def check_config(self, cfg):
        config.load(
            build_config_details({
                'version': '2.3',
                'services': {
                    'web': dict(image='busybox', **cfg)
                },
            }, 'working_dir', 'filename.yml')
        )


class SubnetTest(unittest.TestCase):
    INVALID_SUBNET_TYPES = [
        None,
        False,
        10,
    ]

    INVALID_SUBNET_MAPPINGS = [
        "",
        "192.168.0.1/sdfsdfs",
        "192.168.0.1/",
        "192.168.0.1/33",
        "192.168.0.1/01",
        "192.168.0.1",
        "fe80:0000:0000:0000:0204:61ff:fe9d:f156/sdfsdfs",
        "fe80:0000:0000:0000:0204:61ff:fe9d:f156/",
        "fe80:0000:0000:0000:0204:61ff:fe9d:f156/129",
        "fe80:0000:0000:0000:0204:61ff:fe9d:f156/01",
        "fe80:0000:0000:0000:0204:61ff:fe9d:f156",
        "ge80:0000:0000:0000:0204:61ff:fe9d:f156/128",
        "192.168.0.1/31/31",
    ]

    VALID_SUBNET_MAPPINGS = [
        "192.168.0.1/0",
        "192.168.0.1/32",
        "fe80:0000:0000:0000:0204:61ff:fe9d:f156/0",
        "fe80:0000:0000:0000:0204:61ff:fe9d:f156/128",
        "1:2:3:4:5:6:7:8/0",
        "1::/0",
        "1:2:3:4:5:6:7::/0",
        "1::8/0",
        "1:2:3:4:5:6::8/0",
        "::/0",
        "::8/0",
        "::2:3:4:5:6:7:8/0",
        "fe80::7:8%eth0/0",
        "fe80::7:8%1/0",
        "::255.255.255.255/0",
        "::ffff:255.255.255.255/0",
        "::ffff:0:255.255.255.255/0",
        "2001:db8:3:4::192.0.2.33/0",
        "64:ff9b::192.0.2.33/0",
    ]

    def test_config_invalid_subnet_type_validation(self):
        for invalid_subnet in self.INVALID_SUBNET_TYPES:
            with pytest.raises(ConfigurationError) as exc:
                self.check_config(invalid_subnet)

            assert "contains an invalid type" in exc.value.msg

    def test_config_invalid_subnet_format_validation(self):
        for invalid_subnet in self.INVALID_SUBNET_MAPPINGS:
            with pytest.raises(ConfigurationError) as exc:
                self.check_config(invalid_subnet)

            assert "should use the CIDR format" in exc.value.msg

    def test_config_valid_subnet_format_validation(self):
        for valid_subnet in self.VALID_SUBNET_MAPPINGS:
            self.check_config(valid_subnet)

    def check_config(self, subnet):
        config.load(
            build_config_details({
                'version': '3.5',
                'services': {
                    'web': {
                        'image': 'busybox'
                    }
                },
                'networks': {
                    'default': {
                        'ipam': {
                            'config': [
                                {
                                    'subnet': subnet
                                }
                            ],
                            'driver': 'default'
                        }
                    }
                }
            })
        )


class InterpolationTest(unittest.TestCase):

    @mock.patch.dict(os.environ)
    def test_config_file_with_environment_file(self):
        project_dir = 'tests/fixtures/default-env-file'
        service_dicts = config.load(
            config.find(
                project_dir, None, Environment.from_env_file(project_dir)
            )
        ).services

        assert service_dicts[0] == {
            'name': 'web',
            'image': 'alpine:latest',
            'ports': [
                types.ServicePort.parse('5643')[0],
                types.ServicePort.parse('9999')[0]
            ],
            'command': 'true'
        }

    @mock.patch.dict(os.environ)
    def test_config_file_with_options_environment_file(self):
        project_dir = 'tests/fixtures/default-env-file'
        # env-file is relative to current working dir
        env = Environment.from_env_file(project_dir, project_dir + '/.env2')
        service_dicts = config.load(
            config.find(
                project_dir, None, env
            )
        ).services

        assert service_dicts[0] == {
            'name': 'web',
            'image': 'alpine:latest',
            'ports': [
                types.ServicePort.parse('5644')[0],
                types.ServicePort.parse('9998')[0]
            ],
            'command': 'false'
        }

    @mock.patch.dict(os.environ)
    def test_config_file_with_environment_variable(self):
        project_dir = 'tests/fixtures/environment-interpolation'
        os.environ.update(
            IMAGE="busybox",
            HOST_PORT="80",
            LABEL_VALUE="myvalue",
        )

        service_dicts = config.load(
            config.find(
                project_dir, None, Environment.from_env_file(project_dir)
            )
        ).services

        assert service_dicts == [
            {
                'name': 'web',
                'image': 'busybox',
                'ports': types.ServicePort.parse('80:8000'),
                'labels': {'mylabel': 'myvalue'},
                'hostname': 'host-',
                'command': '${ESCAPED}',
            }
        ]

    @mock.patch.dict(os.environ)
    def test_config_file_with_environment_variable_with_defaults(self):
        project_dir = 'tests/fixtures/environment-interpolation-with-defaults'
        os.environ.update(
            IMAGE="busybox",
        )

        service_dicts = config.load(
            config.find(
                project_dir, None, Environment.from_env_file(project_dir)
            )
        ).services

        assert service_dicts == [
            {
                'name': 'web',
                'image': 'busybox',
                'ports': types.ServicePort.parse('80:8000'),
                'hostname': 'host-',
            }
        ]

    @mock.patch.dict(os.environ)
    def test_unset_variable_produces_warning(self):
        os.environ.pop('FOO', None)
        os.environ.pop('BAR', None)
        config_details = build_config_details(
            {
                'web': {
                    'image': '${FOO}',
                    'command': '${BAR}',
                    'container_name': '${BAR}',
                },
            },
            '.',
            None,
        )

        with mock.patch('compose.config.environment.log') as log:
            config.load(config_details)

            assert 2 == log.warning.call_count
            warnings = sorted(args[0][0] for args in log.warning.call_args_list)
            assert 'BAR' in warnings[0]
            assert 'FOO' in warnings[1]

    @pytest.mark.skip(reason='compatibility mode was removed internally')
    def test_compatibility_mode_warnings(self):
        config_details = build_config_details({
            'version': '3.5',
            'services': {
                'web': {
                    'deploy': {
                        'labels': ['abc=def'],
                        'endpoint_mode': 'dnsrr',
                        'update_config': {'max_failure_ratio': 0.4},
                        'placement': {'constraints': ['node.id==deadbeef']},
                        'resources': {
                            'reservations': {'cpus': '0.2'}
                        },
                        'restart_policy': {
                            'delay': '2s',
                            'window': '12s'
                        }
                    },
                    'image': 'busybox'
                }
            }
        })

        with mock.patch('compose.config.config.log') as log:
            config.load(config_details, compatibility=True)

        assert log.warning.call_count == 1
        warn_message = log.warning.call_args[0][0]
        assert warn_message.startswith(
            'The following deploy sub-keys are not supported in compatibility mode'
        )
        assert 'labels' in warn_message
        assert 'endpoint_mode' in warn_message
        assert 'update_config' in warn_message
        assert 'resources.reservations.cpus' in warn_message
        assert 'restart_policy.delay' in warn_message
        assert 'restart_policy.window' in warn_message

    @pytest.mark.skip(reason='compatibility mode was removed internally')
    def test_compatibility_mode_load(self):
        config_details = build_config_details({
            'version': '3.5',
            'services': {
                'foo': {
                    'image': 'alpine:3.10.1',
                    'deploy': {
                        'replicas': 3,
                        'restart_policy': {
                            'condition': 'any',
                            'max_attempts': 7,
                        },
                        'resources': {
                            'limits': {'memory': '300M', 'cpus': '0.7'},
                            'reservations': {'memory': '100M'},
                        },
                    },
                    'credential_spec': {
                        'file': 'spec.json'
                    },
                },
            },
        })

        with mock.patch('compose.config.config.log') as log:
            cfg = config.load(config_details, compatibility=True)

        assert log.warning.call_count == 0

        service_dict = cfg.services[0]
        assert service_dict == {
            'image': 'alpine:3.10.1',
            'scale': 3,
            'restart': {'MaximumRetryCount': 7, 'Name': 'always'},
            'mem_limit': '300M',
            'mem_reservation': '100M',
            'cpus': 0.7,
            'name': 'foo',
            'security_opt': ['credentialspec=file://spec.json'],
        }

    @mock.patch.dict(os.environ)
    def test_invalid_interpolation(self):
        with pytest.raises(config.ConfigurationError) as cm:
            config.load(
                build_config_details(
                    {'web': {'image': '${'}},
                    'working_dir',
                    'filename.yml'
                )
            )

        assert 'Invalid' in cm.value.msg
        assert 'for "image" option' in cm.value.msg
        assert 'in service "web"' in cm.value.msg
        assert '"${"' in cm.value.msg

    @mock.patch.dict(os.environ)
    def test_interpolation_secrets_section(self):
        os.environ['FOO'] = 'baz.bar'
        config_dict = config.load(build_config_details({
            'version': '3.1',
            'secrets': {
                'secretdata': {
                    'external': {'name': '$FOO'}
                }
            }
        }))
        assert config_dict.secrets == {
            'secretdata': {
                'external': {'name': 'baz.bar'},
                'name': 'baz.bar'
            }
        }

    @mock.patch.dict(os.environ)
    def test_interpolation_configs_section(self):
        os.environ['FOO'] = 'baz.bar'
        config_dict = config.load(build_config_details({
            'version': '3.3',
            'configs': {
                'configdata': {
                    'external': {'name': '$FOO'}
                }
            }
        }))
        assert config_dict.configs == {
            'configdata': {
                'external': {'name': 'baz.bar'},
                'name': 'baz.bar'
            }
        }


class VolumeConfigTest(unittest.TestCase):

    def test_no_binding(self):
        d = make_service_dict('foo', {'build': '.', 'volumes': ['/data']}, working_dir='.')
        assert d['volumes'] == ['/data']

    @mock.patch.dict(os.environ)
    def test_volume_binding_with_environment_variable(self):
        os.environ['VOLUME_PATH'] = '/host/path'

        d = config.load(
            build_config_details(
                {'foo': {'build': '.', 'volumes': ['${VOLUME_PATH}:/container/path']}},
                '.',
                None,
            )
        ).services[0]
        assert d['volumes'] == [VolumeSpec.parse('/host/path:/container/path')]

    @pytest.mark.skipif(IS_WINDOWS_PLATFORM, reason='posix paths')
    def test_volumes_order_is_preserved(self):
        volumes = ['/{0}:/{0}'.format(i) for i in range(0, 6)]
        shuffle(volumes)
        cfg = make_service_dict('foo', {'build': '.', 'volumes': volumes})
        assert cfg['volumes'] == volumes

    @pytest.mark.skipif(IS_WINDOWS_PLATFORM, reason='posix paths')
    @mock.patch.dict(os.environ)
    def test_volume_binding_with_home(self):
        os.environ['HOME'] = '/home/user'
        d = make_service_dict('foo', {'build': '.', 'volumes': ['~:/container/path']}, working_dir='.')
        assert d['volumes'] == ['/home/user:/container/path']

    def test_name_does_not_expand(self):
        d = make_service_dict('foo', {'build': '.', 'volumes': ['mydatavolume:/data']}, working_dir='.')
        assert d['volumes'] == ['mydatavolume:/data']

    def test_absolute_posix_path_does_not_expand(self):
        d = make_service_dict('foo', {'build': '.', 'volumes': ['/var/lib/data:/data']}, working_dir='.')
        assert d['volumes'] == ['/var/lib/data:/data']

    def test_absolute_windows_path_does_not_expand(self):
        d = make_service_dict('foo', {'build': '.', 'volumes': ['c:\\data:/data']}, working_dir='.')
        assert d['volumes'] == ['c:\\data:/data']

    @pytest.mark.skipif(IS_WINDOWS_PLATFORM, reason='posix paths')
    def test_relative_path_does_expand_posix(self):
        d = make_service_dict(
            'foo',
            {'build': '.', 'volumes': ['./data:/data']},
            working_dir='/home/me/myproject')
        assert d['volumes'] == ['/home/me/myproject/data:/data']

        d = make_service_dict(
            'foo',
            {'build': '.', 'volumes': ['.:/data']},
            working_dir='/home/me/myproject')
        assert d['volumes'] == ['/home/me/myproject:/data']

        d = make_service_dict(
            'foo',
            {'build': '.', 'volumes': ['../otherproject:/data']},
            working_dir='/home/me/myproject')
        assert d['volumes'] == ['/home/me/otherproject:/data']

    @pytest.mark.skipif(not IS_WINDOWS_PLATFORM, reason='windows paths')
    def test_relative_path_does_expand_windows(self):
        d = make_service_dict(
            'foo',
            {'build': '.', 'volumes': ['./data:/data']},
            working_dir='c:\\Users\\me\\myproject')
        assert d['volumes'] == ['c:\\Users\\me\\myproject\\data:/data']

        d = make_service_dict(
            'foo',
            {'build': '.', 'volumes': ['.:/data']},
            working_dir='c:\\Users\\me\\myproject')
        assert d['volumes'] == ['c:\\Users\\me\\myproject:/data']

        d = make_service_dict(
            'foo',
            {'build': '.', 'volumes': ['../otherproject:/data']},
            working_dir='c:\\Users\\me\\myproject')
        assert d['volumes'] == ['c:\\Users\\me\\otherproject:/data']

    @mock.patch.dict(os.environ)
    def test_home_directory_with_driver_does_not_expand(self):
        os.environ['NAME'] = 'surprise!'
        d = make_service_dict('foo', {
            'build': '.',
            'volumes': ['~:/data'],
            'volume_driver': 'foodriver',
        }, working_dir='.')
        assert d['volumes'] == ['~:/data']

    def test_volume_path_with_non_ascii_directory(self):
        volume = '/Füü/data:/data'
        container_path = config.resolve_volume_path(".", volume)
        assert container_path == volume


class MergePathMappingTest:
    config_name = ""

    def test_empty(self):
        service_dict = config.merge_service_dicts({}, {}, DEFAULT_VERSION)
        assert self.config_name not in service_dict

    def test_no_override(self):
        service_dict = config.merge_service_dicts(
            {self.config_name: ['/foo:/code', '/data']},
            {},
            DEFAULT_VERSION)
        assert set(service_dict[self.config_name]) == {'/foo:/code', '/data'}

    def test_no_base(self):
        service_dict = config.merge_service_dicts(
            {},
            {self.config_name: ['/bar:/code']},
            DEFAULT_VERSION)
        assert set(service_dict[self.config_name]) == {'/bar:/code'}

    def test_override_explicit_path(self):
        service_dict = config.merge_service_dicts(
            {self.config_name: ['/foo:/code', '/data']},
            {self.config_name: ['/bar:/code']},
            DEFAULT_VERSION)
        assert set(service_dict[self.config_name]) == {'/bar:/code', '/data'}

    def test_add_explicit_path(self):
        service_dict = config.merge_service_dicts(
            {self.config_name: ['/foo:/code', '/data']},
            {self.config_name: ['/bar:/code', '/quux:/data']},
            DEFAULT_VERSION)
        assert set(service_dict[self.config_name]) == {'/bar:/code', '/quux:/data'}

    def test_remove_explicit_path(self):
        service_dict = config.merge_service_dicts(
            {self.config_name: ['/foo:/code', '/quux:/data']},
            {self.config_name: ['/bar:/code', '/data']},
            DEFAULT_VERSION)
        assert set(service_dict[self.config_name]) == {'/bar:/code', '/data'}


class MergeVolumesTest(unittest.TestCase, MergePathMappingTest):
    config_name = 'volumes'


class MergeDevicesTest(unittest.TestCase, MergePathMappingTest):
    config_name = 'devices'


class BuildOrImageMergeTest(unittest.TestCase):

    def test_merge_build_or_image_no_override(self):
        assert config.merge_service_dicts({'build': '.'}, {}, V1) == {'build': '.'}

        assert config.merge_service_dicts({'image': 'redis'}, {}, V1) == {'image': 'redis'}

    def test_merge_build_or_image_override_with_same(self):
        assert config.merge_service_dicts({'build': '.'}, {'build': './web'}, V1) == {'build': './web'}

        assert config.merge_service_dicts({'image': 'redis'}, {'image': 'postgres'}, V1) == {
            'image': 'postgres'
        }

    def test_merge_build_or_image_override_with_other(self):
        assert config.merge_service_dicts({'build': '.'}, {'image': 'redis'}, V1) == {
            'image': 'redis'
        }

        assert config.merge_service_dicts({'image': 'redis'}, {'build': '.'}, V1) == {'build': '.'}


class MergeListsTest:
    config_name = ""
    base_config = []
    override_config = []

    def merged_config(self):
        return set(self.base_config) | set(self.override_config)

    def test_empty(self):
        assert self.config_name not in config.merge_service_dicts({}, {}, DEFAULT_VERSION)

    def test_no_override(self):
        service_dict = config.merge_service_dicts(
            {self.config_name: self.base_config},
            {},
            DEFAULT_VERSION)
        assert set(service_dict[self.config_name]) == set(self.base_config)

    def test_no_base(self):
        service_dict = config.merge_service_dicts(
            {},
            {self.config_name: self.base_config},
            DEFAULT_VERSION)
        assert set(service_dict[self.config_name]) == set(self.base_config)

    def test_add_item(self):
        service_dict = config.merge_service_dicts(
            {self.config_name: self.base_config},
            {self.config_name: self.override_config},
            DEFAULT_VERSION)
        assert set(service_dict[self.config_name]) == set(self.merged_config())


class MergePortsTest(unittest.TestCase, MergeListsTest):
    config_name = 'ports'
    base_config = ['10:8000', '9000']
    override_config = ['20:8000']

    def merged_config(self):
        return self.convert(self.base_config) | self.convert(self.override_config)

    def convert(self, port_config):
        return set(config.merge_service_dicts(
            {self.config_name: port_config},
            {self.config_name: []},
            DEFAULT_VERSION
        )[self.config_name])

    def test_duplicate_port_mappings(self):
        service_dict = config.merge_service_dicts(
            {self.config_name: self.base_config},
            {self.config_name: self.base_config},
            DEFAULT_VERSION
        )
        assert set(service_dict[self.config_name]) == self.convert(self.base_config)

    def test_no_override(self):
        service_dict = config.merge_service_dicts(
            {self.config_name: self.base_config},
            {},
            DEFAULT_VERSION)
        assert set(service_dict[self.config_name]) == self.convert(self.base_config)

    def test_no_base(self):
        service_dict = config.merge_service_dicts(
            {},
            {self.config_name: self.base_config},
            DEFAULT_VERSION)
        assert set(service_dict[self.config_name]) == self.convert(self.base_config)


class MergeNetworksTest(unittest.TestCase, MergeListsTest):
    config_name = 'networks'
    base_config = {'default': {'aliases': ['foo.bar', 'foo.baz']}}
    override_config = {'default': {'ipv4_address': '123.234.123.234'}}

    def test_no_network_overrides(self):
        service_dict = config.merge_service_dicts(
            {self.config_name: self.base_config},
            {self.config_name: self.override_config},
            DEFAULT_VERSION)
        assert service_dict[self.config_name] == {
            'default': {
                'aliases': ['foo.bar', 'foo.baz'],
                'ipv4_address': '123.234.123.234'
            }
        }

    def test_network_has_none_value(self):
        service_dict = config.merge_service_dicts(
            {self.config_name: {
                'default': None
            }},
            {self.config_name: {
                'default': {
                    'aliases': []
                }
            }},
            DEFAULT_VERSION)

        assert service_dict[self.config_name] == {
            'default': {
                'aliases': []
            }
        }

    def test_all_properties(self):
        service_dict = config.merge_service_dicts(
            {self.config_name: {
                'default': {
                    'aliases': ['foo.bar', 'foo.baz'],
                    'link_local_ips': ['192.168.1.10', '192.168.1.11'],
                    'ipv4_address': '111.111.111.111',
                    'ipv6_address': 'FE80:CD00:0000:0CDE:1257:0000:211E:729C-first'
                }
            }},
            {self.config_name: {
                'default': {
                    'aliases': ['foo.baz', 'foo.baz2'],
                    'link_local_ips': ['192.168.1.11', '192.168.1.12'],
                    'ipv4_address': '123.234.123.234',
                    'ipv6_address': 'FE80:CD00:0000:0CDE:1257:0000:211E:729C-second'
                }
            }},
            DEFAULT_VERSION)

        assert service_dict[self.config_name] == {
            'default': {
                'aliases': ['foo.bar', 'foo.baz', 'foo.baz2'],
                'link_local_ips': ['192.168.1.10', '192.168.1.11', '192.168.1.12'],
                'ipv4_address': '123.234.123.234',
                'ipv6_address': 'FE80:CD00:0000:0CDE:1257:0000:211E:729C-second'
            }
        }

    def test_no_network_name_overrides(self):
        service_dict = config.merge_service_dicts(
            {
                self.config_name: {
                    'default': {
                        'aliases': ['foo.bar', 'foo.baz'],
                        'ipv4_address': '123.234.123.234'
                    }
                }
            },
            {
                self.config_name: {
                    'another_network': {
                        'ipv4_address': '123.234.123.234'
                    }
                }
            },
            DEFAULT_VERSION)
        assert service_dict[self.config_name] == {
            'default': {
                'aliases': ['foo.bar', 'foo.baz'],
                'ipv4_address': '123.234.123.234'
            },
            'another_network': {
                'ipv4_address': '123.234.123.234'
            }
        }


class MergeStringsOrListsTest(unittest.TestCase):

    def test_no_override(self):
        service_dict = config.merge_service_dicts(
            {'dns': '8.8.8.8'},
            {},
            DEFAULT_VERSION)
        assert set(service_dict['dns']) == {'8.8.8.8'}

    def test_no_base(self):
        service_dict = config.merge_service_dicts(
            {},
            {'dns': '8.8.8.8'},
            DEFAULT_VERSION)
        assert set(service_dict['dns']) == {'8.8.8.8'}

    def test_add_string(self):
        service_dict = config.merge_service_dicts(
            {'dns': ['8.8.8.8']},
            {'dns': '9.9.9.9'},
            DEFAULT_VERSION)
        assert set(service_dict['dns']) == {'8.8.8.8', '9.9.9.9'}

    def test_add_list(self):
        service_dict = config.merge_service_dicts(
            {'dns': '8.8.8.8'},
            {'dns': ['9.9.9.9']},
            DEFAULT_VERSION)
        assert set(service_dict['dns']) == {'8.8.8.8', '9.9.9.9'}


class MergeLabelsTest(unittest.TestCase):

    def test_empty(self):
        assert 'labels' not in config.merge_service_dicts({}, {}, DEFAULT_VERSION)

    def test_no_override(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'build': '.', 'labels': ['foo=1', 'bar']}, 'tests/'),
            make_service_dict('foo', {'build': '.'}, 'tests/'),
            DEFAULT_VERSION)
        assert service_dict['labels'] == {'foo': '1', 'bar': ''}

    def test_no_base(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'build': '.'}, 'tests/'),
            make_service_dict('foo', {'build': '.', 'labels': ['foo=2']}, 'tests/'),
            DEFAULT_VERSION)
        assert service_dict['labels'] == {'foo': '2'}

    def test_override_explicit_value(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'build': '.', 'labels': ['foo=1', 'bar']}, 'tests/'),
            make_service_dict('foo', {'build': '.', 'labels': ['foo=2']}, 'tests/'),
            DEFAULT_VERSION)
        assert service_dict['labels'] == {'foo': '2', 'bar': ''}

    def test_add_explicit_value(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'build': '.', 'labels': ['foo=1', 'bar']}, 'tests/'),
            make_service_dict('foo', {'build': '.', 'labels': ['bar=2']}, 'tests/'),
            DEFAULT_VERSION)
        assert service_dict['labels'] == {'foo': '1', 'bar': '2'}

    def test_remove_explicit_value(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'build': '.', 'labels': ['foo=1', 'bar=2']}, 'tests/'),
            make_service_dict('foo', {'build': '.', 'labels': ['bar']}, 'tests/'),
            DEFAULT_VERSION)
        assert service_dict['labels'] == {'foo': '1', 'bar': ''}


class MergeBuildTest(unittest.TestCase):
    def test_full(self):
        base = {
            'context': '.',
            'dockerfile': 'Dockerfile',
            'args': {
                'x': '1',
                'y': '2',
            },
            'cache_from': ['ubuntu'],
            'labels': ['com.docker.compose.test=true']
        }

        override = {
            'context': './prod',
            'dockerfile': 'Dockerfile.prod',
            'args': ['x=12'],
            'cache_from': ['debian'],
            'labels': {
                'com.docker.compose.test': 'false',
                'com.docker.compose.prod': 'true',
            }
        }

        result = config.merge_build(None, {'build': base}, {'build': override})
        assert result['context'] == override['context']
        assert result['dockerfile'] == override['dockerfile']
        assert result['args'] == {'x': '12', 'y': '2'}
        assert set(result['cache_from']) == {'ubuntu', 'debian'}
        assert result['labels'] == override['labels']

    def test_empty_override(self):
        base = {
            'context': '.',
            'dockerfile': 'Dockerfile',
            'args': {
                'x': '1',
                'y': '2',
            },
            'cache_from': ['ubuntu'],
            'labels': {
                'com.docker.compose.test': 'true'
            }
        }

        override = {}

        result = config.merge_build(None, {'build': base}, {'build': override})
        assert result == base

    def test_empty_base(self):
        base = {}

        override = {
            'context': './prod',
            'dockerfile': 'Dockerfile.prod',
            'args': {'x': '12'},
            'cache_from': ['debian'],
            'labels': {
                'com.docker.compose.test': 'false',
                'com.docker.compose.prod': 'true',
            }
        }

        result = config.merge_build(None, {'build': base}, {'build': override})
        assert result == override


class MemoryOptionsTest(unittest.TestCase):

    def test_validation_fails_with_just_memswap_limit(self):
        """
        When you set a 'memswap_limit' it is invalid config unless you also set
        a mem_limit
        """
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'foo': {'image': 'busybox', 'memswap_limit': 2000000},
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "foo.memswap_limit is invalid: when defining " \
               "'memswap_limit' you must set 'mem_limit' as well" \
            in excinfo.exconly()

    def test_validation_with_correct_memswap_values(self):
        service_dict = config.load(
            build_config_details(
                {'foo': {'image': 'busybox', 'mem_limit': 1000000, 'memswap_limit': 2000000}},
                'tests/fixtures/extends',
                'common.yml'
            )
        ).services
        assert service_dict[0]['memswap_limit'] == 2000000

    def test_memswap_can_be_a_string(self):
        service_dict = config.load(
            build_config_details(
                {'foo': {'image': 'busybox', 'mem_limit': "1G", 'memswap_limit': "512M"}},
                'tests/fixtures/extends',
                'common.yml'
            )
        ).services
        assert service_dict[0]['memswap_limit'] == "512M"


class EnvTest(unittest.TestCase):

    def test_parse_environment_as_list(self):
        environment = [
            'NORMAL=F1',
            'CONTAINS_EQUALS=F=2',
            'TRAILING_EQUALS=',
        ]
        assert config.parse_environment(environment) == {
            'NORMAL': 'F1', 'CONTAINS_EQUALS': 'F=2', 'TRAILING_EQUALS': ''
        }

    def test_parse_environment_as_dict(self):
        environment = {
            'NORMAL': 'F1',
            'CONTAINS_EQUALS': 'F=2',
            'TRAILING_EQUALS': None,
        }
        assert config.parse_environment(environment) == environment

    def test_parse_environment_invalid(self):
        with pytest.raises(ConfigurationError):
            config.parse_environment('a=b')

    def test_parse_environment_empty(self):
        assert config.parse_environment(None) == {}

    @mock.patch.dict(os.environ)
    def test_resolve_environment(self):
        os.environ['FILE_DEF'] = 'E1'
        os.environ['FILE_DEF_EMPTY'] = 'E2'
        os.environ['ENV_DEF'] = 'E3'

        service_dict = {
            'build': '.',
            'environment': {
                'FILE_DEF': 'F1',
                'FILE_DEF_EMPTY': '',
                'ENV_DEF': None,
                'NO_DEF': None
            },
        }
        assert resolve_environment(
            service_dict, Environment.from_env_file(None)
        ) == {'FILE_DEF': 'F1', 'FILE_DEF_EMPTY': '', 'ENV_DEF': 'E3', 'NO_DEF': None}

    def test_resolve_environment_from_env_file(self):
        assert resolve_environment({'env_file': ['tests/fixtures/env/one.env']}) == {
            'ONE': '2', 'TWO': '1', 'THREE': '3', 'FOO': 'bar'
        }

    def test_environment_overrides_env_file(self):
        assert resolve_environment({
            'environment': {'FOO': 'baz'},
            'env_file': ['tests/fixtures/env/one.env'],
        }) == {'ONE': '2', 'TWO': '1', 'THREE': '3', 'FOO': 'baz'}

    def test_resolve_environment_with_multiple_env_files(self):
        service_dict = {
            'env_file': [
                'tests/fixtures/env/one.env',
                'tests/fixtures/env/two.env'
            ]
        }
        assert resolve_environment(service_dict) == {
            'ONE': '2', 'TWO': '1', 'THREE': '3', 'FOO': 'baz', 'DOO': 'dah'
        }

    def test_resolve_environment_nonexistent_file(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details(
                {'foo': {'image': 'example', 'env_file': 'nonexistent.env'}},
                working_dir='tests/fixtures/env'))

        assert 'Couldn\'t find env file' in exc.exconly()
        assert 'nonexistent.env' in exc.exconly()

    @mock.patch.dict(os.environ)
    def test_resolve_environment_from_env_file_with_empty_values(self):
        os.environ['FILE_DEF'] = 'E1'
        os.environ['FILE_DEF_EMPTY'] = 'E2'
        os.environ['ENV_DEF'] = 'E3'
        assert resolve_environment(
            {'env_file': ['tests/fixtures/env/resolve.env']},
            Environment.from_env_file(None)
        ) == {
            'FILE_DEF': 'bär',
            'FILE_DEF_EMPTY': '',
            'ENV_DEF': 'E3',
            'NO_DEF': None
        }

    @mock.patch.dict(os.environ)
    def test_resolve_build_args(self):
        os.environ['env_arg'] = 'value2'

        build = {
            'context': '.',
            'args': {
                'arg1': 'value1',
                'empty_arg': '',
                'env_arg': None,
                'no_env': None
            }
        }
        assert resolve_build_args(build['args'], Environment.from_env_file(build['context'])) == {
            'arg1': 'value1', 'empty_arg': '', 'env_arg': 'value2', 'no_env': None
        }

    @pytest.mark.xfail(IS_WINDOWS_PLATFORM, reason='paths use slash')
    @mock.patch.dict(os.environ)
    def test_resolve_path(self):
        os.environ['HOSTENV'] = '/tmp'
        os.environ['CONTAINERENV'] = '/host/tmp'

        service_dict = config.load(
            build_config_details(
                {'services': {
                    'foo': {'build': '.', 'volumes': ['$HOSTENV:$CONTAINERENV']}}},
                "tests/fixtures/env",
            )
        ).services[0]
        assert set(service_dict['volumes']) == {VolumeSpec.parse('/tmp:/host/tmp')}

        service_dict = config.load(
            build_config_details(
                {'services': {
                    'foo': {'build': '.', 'volumes': ['/opt${HOSTENV}:/opt${CONTAINERENV}']}}},
                "tests/fixtures/env",
            )
        ).services[0]
        assert set(service_dict['volumes']) == {VolumeSpec.parse('/opt/tmp:/opt/host/tmp')}


def load_from_filename(filename, override_dir=None):
    return config.load(
        config.find('.', [filename], Environment.from_env_file('.'), override_dir=override_dir)
    ).services


class ExtendsTest(unittest.TestCase):

    def test_extends(self):
        service_dicts = load_from_filename('tests/fixtures/extends/docker-compose.yml')

        assert service_sort(service_dicts) == service_sort([
            {
                'name': 'mydb',
                'image': 'busybox',
                'command': 'top',
            },
            {
                'name': 'myweb',
                'image': 'busybox',
                'command': 'top',
                'network_mode': 'bridge',
                'links': ['mydb:db'],
                'environment': {
                    "FOO": "1",
                    "BAR": "2",
                    "BAZ": "2",
                },
            }
        ])

    def test_merging_env_labels_ulimits(self):
        service_dicts = load_from_filename('tests/fixtures/extends/common-env-labels-ulimits.yml')

        assert service_sort(service_dicts) == service_sort([
            {
                'name': 'web',
                'image': 'busybox',
                'command': '/bin/true',
                'network_mode': 'host',
                'environment': {
                    "FOO": "2",
                    "BAR": "1",
                    "BAZ": "3",
                },
                'labels': {'label': 'one'},
                'ulimits': {'nproc': 65535, 'memlock': {'soft': 1024, 'hard': 2048}}
            }
        ])

    def test_nested(self):
        service_dicts = load_from_filename('tests/fixtures/extends/nested.yml')

        assert service_dicts == [
            {
                'name': 'myweb',
                'image': 'busybox',
                'command': '/bin/true',
                'network_mode': 'host',
                'environment': {
                    "FOO": "2",
                    "BAR": "2",
                },
            },
        ]

    def test_self_referencing_file(self):
        """
        We specify a 'file' key that is the filename we're already in.
        """
        service_dicts = load_from_filename('tests/fixtures/extends/specify-file-as-self.yml')
        assert service_sort(service_dicts) == service_sort([
            {
                'environment':
                {
                    'YEP': '1', 'BAR': '1', 'BAZ': '3'
                },
                'image': 'busybox',
                'name': 'myweb'
            },
            {
                'environment':
                {'YEP': '1'},
                'image': 'busybox',
                'name': 'otherweb'
            },
            {
                'environment':
                {'YEP': '1', 'BAZ': '3'},
                'image': 'busybox',
                'name': 'web'
            }
        ])

    def test_circular(self):
        with pytest.raises(config.CircularReference) as exc:
            load_from_filename('tests/fixtures/extends/circle-1.yml')

        path = [
            (os.path.basename(filename), service_name)
            for (filename, service_name) in exc.value.trail
        ]
        expected = [
            ('circle-1.yml', 'web'),
            ('circle-2.yml', 'other'),
            ('circle-1.yml', 'web'),
        ]
        assert path == expected

    def test_extends_validation_empty_dictionary(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': '3',
                        'services':
                        {
                            'web': {'image': 'busybox', 'extends': {}},
                        }
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert 'service' in excinfo.exconly()

    def test_extends_validation_missing_service_key(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': '3',
                        'services':
                        {
                            'web': {
                                'image': 'busybox',
                                'extends': {'file': 'common.yml'}
                            }
                        }
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "'service' is a required property" in excinfo.exconly()

    def test_extends_validation_invalid_key(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': '3',
                        'services':
                        {
                            'web': {
                                'image': 'busybox',
                                'extends': {
                                    'file': 'common.yml',
                                    'service': 'web',
                                    'rogue_key': 'is not allowed'
                                }
                            },
                        }
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "web.extends contains unsupported option: 'rogue_key'" \
            in excinfo.exconly()

    def test_extends_validation_sub_property_key(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details(
                    {
                        'version': '3',
                        'services': {
                            'web': {
                                'image': 'busybox',
                                'extends': {
                                    'file': 1,
                                    'service': 'web',
                                }
                            }
                        },
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

        assert "web.extends.file contains 1, which is an invalid type, it should be a string" \
            in excinfo.exconly()

    def test_extends_validation_no_file_key_no_filename_set(self):
        dictionary = {'extends': {'service': 'web'}}

        with pytest.raises(ConfigurationError) as excinfo:
            make_service_dict('myweb', dictionary, working_dir='tests/fixtures/extends')

        assert 'file' in excinfo.exconly()

    def test_extends_validation_valid_config(self):
        service = config.load(
            build_config_details(
                {
                    'web': {'image': 'busybox', 'extends': {'service': 'web', 'file': 'common.yml'}},
                },
                'tests/fixtures/extends',
                'common.yml'
            )
        ).services

        assert len(service) == 1
        assert isinstance(service[0], dict)
        assert service[0]['command'] == "/bin/true"

    def test_extended_service_with_invalid_config(self):
        with pytest.raises(ConfigurationError) as exc:
            load_from_filename('tests/fixtures/extends/service-with-invalid-schema.yml')
        assert (
            "myweb has neither an image nor a build context specified" in
            exc.exconly()
        )

    def test_extended_service_with_valid_config(self):
        service = load_from_filename('tests/fixtures/extends/service-with-valid-composite-extends.yml')
        assert service[0]['command'] == "top"

    def test_extends_file_defaults_to_self(self):
        """
        Test not specifying a file in our extends options that the
        config is valid and correctly extends from itself.
        """
        service_dicts = load_from_filename('tests/fixtures/extends/no-file-specified.yml')
        assert service_sort(service_dicts) == service_sort([
            {
                'name': 'myweb',
                'image': 'busybox',
                'environment': {
                    "BAR": "1",
                    "BAZ": "3",
                }
            },
            {
                'name': 'web',
                'image': 'busybox',
                'environment': {
                    "BAZ": "3",
                }
            }
        ])

    def test_invalid_links_in_extended_service(self):
        with pytest.raises(ConfigurationError) as excinfo:
            load_from_filename('tests/fixtures/extends/invalid-links.yml')

        assert "services with 'links' cannot be extended" in excinfo.exconly()

    def test_invalid_volumes_from_in_extended_service(self):
        with pytest.raises(ConfigurationError) as excinfo:
            load_from_filename('tests/fixtures/extends/invalid-volumes.yml')

        assert "services with 'volumes_from' cannot be extended" in excinfo.exconly()

    def test_invalid_net_in_extended_service(self):
        with pytest.raises(ConfigurationError) as excinfo:
            load_from_filename('tests/fixtures/extends/invalid-net-v2.yml')

        assert 'network_mode: service' in excinfo.exconly()
        assert 'cannot be extended' in excinfo.exconly()

        with pytest.raises(ConfigurationError) as excinfo:
            load_from_filename('tests/fixtures/extends/invalid-net.yml')

        assert 'net: container' in excinfo.exconly()
        assert 'cannot be extended' in excinfo.exconly()

    @mock.patch.dict(os.environ)
    def test_load_config_runs_interpolation_in_extended_service(self):
        os.environ.update(HOSTNAME_VALUE="penguin")
        expected_interpolated_value = "host-penguin"
        service_dicts = load_from_filename(
            'tests/fixtures/extends/valid-interpolation.yml')
        for service in service_dicts:
            assert service['hostname'] == expected_interpolated_value

    @pytest.mark.xfail(IS_WINDOWS_PLATFORM, reason='paths use slash')
    def test_volume_path(self):
        dicts = load_from_filename('tests/fixtures/volume-path/docker-compose.yml')

        paths = [
            VolumeSpec(
                os.path.abspath('tests/fixtures/volume-path/common/foo'),
                '/foo',
                'rw'),
            VolumeSpec(
                os.path.abspath('tests/fixtures/volume-path/bar'),
                '/bar',
                'rw')
        ]

        assert set(dicts[0]['volumes']) == set(paths)

    def test_parent_build_path_dne(self):
        child = load_from_filename('tests/fixtures/extends/nonexistent-path-child.yml')

        assert child == [
            {
                'name': 'dnechild',
                'image': 'busybox',
                'command': '/bin/true',
                'environment': {
                    "FOO": "1",
                    "BAR": "2",
                },
            },
        ]

    def test_load_throws_error_when_base_service_does_not_exist(self):
        with pytest.raises(ConfigurationError) as excinfo:
            load_from_filename('tests/fixtures/extends/nonexistent-service.yml')

        assert "Cannot extend service 'foo'" in excinfo.exconly()
        assert "Service not found" in excinfo.exconly()

    def test_partial_service_config_in_extends_is_still_valid(self):
        dicts = load_from_filename('tests/fixtures/extends/valid-common-config.yml')
        assert dicts[0]['environment'] == {'FOO': '1'}

    def test_extended_service_with_verbose_and_shorthand_way(self):
        services = load_from_filename('tests/fixtures/extends/verbose-and-shorthand.yml')
        assert service_sort(services) == service_sort([
            {
                'name': 'base',
                'image': 'busybox',
                'environment': {'BAR': '1'},
            },
            {
                'name': 'verbose',
                'image': 'busybox',
                'environment': {'BAR': '1', 'FOO': '1'},
            },
            {
                'name': 'shorthand',
                'image': 'busybox',
                'environment': {'BAR': '1', 'FOO': '2'},
            },
        ])

    @mock.patch.dict(os.environ)
    def test_extends_with_environment_and_env_files(self):
        tmpdir = tempfile.mkdtemp('test_extends_with_environment')
        self.addCleanup(shutil.rmtree, tmpdir)
        commondir = os.path.join(tmpdir, 'common')
        os.mkdir(commondir)
        with open(os.path.join(commondir, 'base.yml'), mode="w") as base_fh:
            base_fh.write("""
                app:
                    image: 'example/app'
                    env_file:
                        - 'envs'
                    environment:
                        - SECRET
                        - TEST_ONE=common
                        - TEST_TWO=common
            """)
        with open(os.path.join(tmpdir, 'docker-compose.yml'), mode="w") as docker_compose_fh:
            docker_compose_fh.write("""
                ext:
                    extends:
                        file: common/base.yml
                        service: app
                    env_file:
                        - 'envs'
                    environment:
                        - THING
                        - TEST_ONE=top
            """)
        with open(os.path.join(commondir, 'envs'), mode="w") as envs_fh:
            envs_fh.write("""
                COMMON_ENV_FILE
                TEST_ONE=common-env-file
                TEST_TWO=common-env-file
                TEST_THREE=common-env-file
                TEST_FOUR=common-env-file
            """)
        with open(os.path.join(tmpdir, 'envs'), mode="w") as envs_fh:
            envs_fh.write("""
                TOP_ENV_FILE
                TEST_ONE=top-env-file
                TEST_TWO=top-env-file
                TEST_THREE=top-env-file
            """)

        expected = [
            {
                'name': 'ext',
                'image': 'example/app',
                'environment': {
                    'SECRET': 'secret',
                    'TOP_ENV_FILE': 'secret',
                    'COMMON_ENV_FILE': 'secret',
                    'THING': 'thing',
                    'TEST_ONE': 'top',
                    'TEST_TWO': 'common',
                    'TEST_THREE': 'top-env-file',
                    'TEST_FOUR': 'common-env-file',
                },
            },
        ]

        os.environ['SECRET'] = 'secret'
        os.environ['THING'] = 'thing'
        os.environ['COMMON_ENV_FILE'] = 'secret'
        os.environ['TOP_ENV_FILE'] = 'secret'
        config = load_from_filename(str(os.path.join(tmpdir, 'docker-compose.yml')))

        assert config == expected

    def test_extends_with_mixed_versions_is_error(self):
        tmpdir = tempfile.mkdtemp('test_extends_with_mixed_version')
        self.addCleanup(shutil.rmtree, tmpdir)
        with open(os.path.join(tmpdir, 'docker-compose.yml'), mode="w") as docker_compose_fh:
            docker_compose_fh.write("""
                version: "2"
                services:
                  web:
                    extends:
                      file: base.yml
                      service: base
                    image: busybox
            """)
        with open(os.path.join(tmpdir, 'base.yml'), mode="w") as base_fh:
            base_fh.write("""
                base:
                  volumes: ['/foo']
                  ports: ['3000:3000']
            """)

        with pytest.raises(ConfigurationError) as exc:
            load_from_filename(str(os.path.join(tmpdir, 'docker-compose.yml')))
        assert 'Version mismatch' in exc.exconly()

    def test_extends_with_defined_version_passes(self):
        tmpdir = tempfile.mkdtemp('test_extends_with_defined_version')
        self.addCleanup(shutil.rmtree, tmpdir)
        with open(os.path.join(tmpdir, 'docker-compose.yml'), mode="w") as docker_compose_fh:
            docker_compose_fh.write("""
                version: "2"
                services:
                  web:
                    extends:
                      file: base.yml
                      service: base
                    image: busybox
            """)
        with open(os.path.join(tmpdir, 'base.yml'), mode="w") as base_fh:
            base_fh.write("""
                version: "2"
                services:
                  base:
                    volumes: ['/foo']
                    ports: ['3000:3000']
                    command: top
            """)

        service = load_from_filename(str(os.path.join(tmpdir, 'docker-compose.yml')))
        assert service[0]['command'] == "top"

    def test_extends_with_depends_on(self):
        tmpdir = tempfile.mkdtemp('test_extends_with_depends_on')
        self.addCleanup(shutil.rmtree, tmpdir)
        with open(os.path.join(tmpdir, 'docker-compose.yml'), mode="w") as docker_compose_fh:
            docker_compose_fh.write("""
                version: "2"
                services:
                  base:
                    image: example
                  web:
                    extends: base
                    image: busybox
                    depends_on: ['other']
                  other:
                    image: example
            """)
        services = load_from_filename(str(os.path.join(tmpdir, 'docker-compose.yml')))
        assert service_sort(services)[2]['depends_on'] == {
            'other': {'condition': 'service_started'}
        }

    def test_extends_with_healthcheck(self):
        service_dicts = load_from_filename('tests/fixtures/extends/healthcheck-2.yml')
        assert service_sort(service_dicts) == [{
            'name': 'demo',
            'image': 'foobar:latest',
            'healthcheck': {
                'test': ['CMD', '/health.sh'],
                'interval': 10000000000,
                'timeout': 5000000000,
                'retries': 36,
            }
        }]

    def test_extends_with_ports(self):
        tmpdir = tempfile.mkdtemp('test_extends_with_ports')
        self.addCleanup(shutil.rmtree, tmpdir)
        with open(os.path.join(tmpdir, 'docker-compose.yml'), mode="w") as docker_compose_fh:
            docker_compose_fh.write("""
                version: '2'

                services:
                  a:
                    image: nginx
                    ports:
                      - 80

                  b:
                    extends:
                      service: a
            """)
        services = load_from_filename(str(os.path.join(tmpdir, 'docker-compose.yml')))

        assert len(services) == 2
        for svc in services:
            assert svc['ports'] == [types.ServicePort('80', None, None, None, None)]

    def test_extends_with_security_opt(self):
        tmpdir = tempfile.mkdtemp('test_extends_with_ports')
        self.addCleanup(shutil.rmtree, tmpdir)
        with open(os.path.join(tmpdir, 'docker-compose.yml'), mode="w") as docker_compose_fh:
            docker_compose_fh.write("""
                version: '2'

                services:
                  a:
                    image: nginx
                    security_opt:
                    - apparmor:unconfined
                    - seccomp:unconfined

                  b:
                    extends:
                      service: a
            """)
        services = load_from_filename(str(os.path.join(tmpdir, 'docker-compose.yml')))
        assert len(services) == 2
        for svc in services:
            assert types.SecurityOpt.parse('apparmor:unconfined') in svc['security_opt']
            assert types.SecurityOpt.parse('seccomp:unconfined') in svc['security_opt']

    @mock.patch.object(ConfigFile, 'from_filename', wraps=ConfigFile.from_filename)
    def test_extends_same_file_optimization(self, from_filename_mock):
        load_from_filename('tests/fixtures/extends/no-file-specified.yml')
        from_filename_mock.assert_called_once()


@pytest.mark.xfail(IS_WINDOWS_PLATFORM, reason='paths use slash')
class ExpandPathTest(unittest.TestCase):
    working_dir = '/home/user/somedir'

    def test_expand_path_normal(self):
        result = config.expand_path(self.working_dir, 'myfile')
        assert result == self.working_dir + '/' + 'myfile'

    def test_expand_path_absolute(self):
        abs_path = '/home/user/otherdir/somefile'
        result = config.expand_path(self.working_dir, abs_path)
        assert result == abs_path

    def test_expand_path_with_tilde(self):
        test_path = '~/otherdir/somefile'
        with mock.patch.dict(os.environ):
            os.environ['HOME'] = user_path = '/home/user/'
            result = config.expand_path(self.working_dir, test_path)

        assert result == user_path + 'otherdir/somefile'


class VolumePathTest(unittest.TestCase):

    def test_split_path_mapping_with_windows_path(self):
        host_path = "c:\\Users\\msamblanet\\Documents\\anvil\\connect\\config"
        windows_volume_path = host_path + ":/opt/connect/config:ro"
        expected_mapping = ("/opt/connect/config", (host_path, 'ro'))

        mapping = config.split_path_mapping(windows_volume_path)
        assert mapping == expected_mapping

    def test_split_path_mapping_with_windows_path_in_container(self):
        host_path = 'c:\\Users\\remilia\\data'
        container_path = 'c:\\scarletdevil\\data'
        expected_mapping = (container_path, (host_path, None))

        mapping = config.split_path_mapping('{}:{}'.format(host_path, container_path))
        assert mapping == expected_mapping

    def test_split_path_mapping_with_root_mount(self):
        host_path = '/'
        container_path = '/var/hostroot'
        expected_mapping = (container_path, (host_path, None))
        mapping = config.split_path_mapping('{}:{}'.format(host_path, container_path))
        assert mapping == expected_mapping


@pytest.mark.xfail(IS_WINDOWS_PLATFORM, reason='paths use slash')
class BuildPathTest(unittest.TestCase):

    def setUp(self):
        self.abs_context_path = os.path.join(os.getcwd(), 'tests/fixtures/build-ctx')

    def test_nonexistent_path(self):
        with pytest.raises(ConfigurationError):
            config.load(
                build_config_details(
                    {
                        'foo': {'build': 'nonexistent.path'},
                    },
                    'working_dir',
                    'filename.yml'
                )
            )

    def test_relative_path(self):
        relative_build_path = '../build-ctx/'
        service_dict = make_service_dict(
            'relpath',
            {'build': relative_build_path},
            working_dir='tests/fixtures/build-path'
        )
        assert service_dict['build'] == self.abs_context_path

    def test_absolute_path(self):
        service_dict = make_service_dict(
            'abspath',
            {'build': self.abs_context_path},
            working_dir='tests/fixtures/build-path'
        )
        assert service_dict['build'] == self.abs_context_path

    def test_from_file(self):
        service_dict = load_from_filename('tests/fixtures/build-path/docker-compose.yml')
        assert service_dict == [{'name': 'foo', 'build': {'context': self.abs_context_path}}]

    def test_from_file_override_dir(self):
        override_dir = os.path.join(os.getcwd(), 'tests/fixtures/')
        service_dict = load_from_filename(
            'tests/fixtures/build-path-override-dir/docker-compose.yml', override_dir=override_dir)
        assert service_dict == [{'name': 'foo', 'build': {'context': self.abs_context_path}}]

    def test_valid_url_in_build_path(self):
        valid_urls = [
            'git://github.com/docker/docker',
            'git@github.com:docker/docker.git',
            'git@bitbucket.org:atlassianlabs/atlassian-docker.git',
            'https://github.com/docker/docker.git',
            'http://github.com/docker/docker.git',
            'github.com/docker/docker.git',
        ]
        for valid_url in valid_urls:
            service_dict = config.load(build_config_details({
                'validurl': {'build': valid_url},
            }, '.', None)).services
            assert service_dict[0]['build'] == {'context': valid_url}

    def test_invalid_url_in_build_path(self):
        invalid_urls = [
            'example.com/bogus',
            'ftp://example.com/',
            '/path/does/not/exist',
        ]
        for invalid_url in invalid_urls:
            with pytest.raises(ConfigurationError) as exc:
                config.load(build_config_details({
                    'invalidurl': {'build': invalid_url},
                }, '.', None))
            assert 'build path' in exc.exconly()


class HealthcheckTest(unittest.TestCase):
    def test_healthcheck(self):
        config_dict = config.load(
            build_config_details({
                'version': '2.3',
                'services': {
                    'test': {
                        'image': 'busybox',
                        'healthcheck': {
                            'test': ['CMD', 'true'],
                            'interval': '1s',
                            'timeout': '1m',
                            'retries': 3,
                            'start_period': '10s',
                        }
                    }
                }

            })
        )

        serialized_config = yaml.safe_load(serialize_config(config_dict))
        serialized_service = serialized_config['services']['test']

        assert serialized_service['healthcheck'] == {
            'test': ['CMD', 'true'],
            'interval': '1s',
            'timeout': '1m',
            'retries': 3,
            'start_period': '10s'
        }

    def test_disable(self):
        config_dict = config.load(
            build_config_details({
                'version': '2.3',
                'services': {
                    'test': {
                        'image': 'busybox',
                        'healthcheck': {
                            'disable': True,
                        }
                    }
                }

            })
        )

        serialized_config = yaml.safe_load(serialize_config(config_dict))
        serialized_service = serialized_config['services']['test']

        assert serialized_service['healthcheck'] == {
            'test': ['NONE'],
        }

    def test_disable_with_other_config_is_invalid(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details({
                    'version': '2.3',
                    'services': {
                        'invalid-healthcheck': {
                            'image': 'busybox',
                            'healthcheck': {
                                'disable': True,
                                'interval': '1s',
                            }
                        }
                    }

                })
            )

        assert 'invalid-healthcheck' in excinfo.exconly()
        assert '"disable: true" cannot be combined with other options' in excinfo.exconly()

    def test_healthcheck_with_invalid_test(self):
        with pytest.raises(ConfigurationError) as excinfo:
            config.load(
                build_config_details({
                    'version': '2.3',
                    'services': {
                        'invalid-healthcheck': {
                            'image': 'busybox',
                            'healthcheck': {
                                'test': ['true'],
                                'interval': '1s',
                                'timeout': '1m',
                                'retries': 3,
                                'start_period': '10s',
                            }
                        }
                    }

                })
            )

        assert 'invalid-healthcheck' in excinfo.exconly()
        assert 'the first item must be either NONE, CMD or CMD-SHELL' in excinfo.exconly()


class GetDefaultConfigFilesTestCase(unittest.TestCase):

    files = [
        'docker-compose.yml',
        'docker-compose.yaml',
        'compose.yml',
        'compose.yaml',
    ]

    def test_get_config_path_default_file_in_basedir(self):
        for index, filename in enumerate(self.files):
            assert filename == get_config_filename_for_files(self.files[index:])
        with pytest.raises(config.ComposeFileNotFound):
            get_config_filename_for_files([])

    def test_get_config_path_default_file_in_parent_dir(self):
        """Test with files placed in the subdir"""

        def get_config_in_subdir(files):
            return get_config_filename_for_files(files, subdir=True)

        for index, filename in enumerate(self.files):
            assert filename == get_config_in_subdir(self.files[index:])
        with pytest.raises(config.ComposeFileNotFound):
            get_config_in_subdir([])


def get_config_filename_for_files(filenames, subdir=None):
    def make_files(dirname, filenames):
        for fname in filenames:
            with open(os.path.join(dirname, fname), 'w') as f:
                f.write('')

    project_dir = tempfile.mkdtemp()
    try:
        make_files(project_dir, filenames)
        if subdir:
            base_dir = tempfile.mkdtemp(dir=project_dir)
        else:
            base_dir = project_dir
        filenames = config.get_default_config_files(base_dir)
        if not filenames:
            raise config.ComposeFileNotFound(config.SUPPORTED_FILENAMES)
        return os.path.basename(filenames[0])
    finally:
        shutil.rmtree(project_dir)


class SerializeTest(unittest.TestCase):
    def test_denormalize_depends(self):
        service_dict = {
            'image': 'busybox',
            'command': 'true',
            'depends_on': {
                'service2': {'condition': 'service_started'},
                'service3': {'condition': 'service_started'},
            }
        }

        assert denormalize_service_dict(service_dict, VERSION) == service_dict

    def test_serialize_time(self):
        data = {
            9: '9ns',
            9000: '9us',
            9000000: '9ms',
            90000000: '90ms',
            900000000: '900ms',
            999999999: '999999999ns',
            1000000000: '1s',
            60000000000: '1m',
            60000000001: '60000000001ns',
            9000000000000: '150m',
            90000000000000: '25h',
        }

        for k, v in data.items():
            assert serialize_ns_time_value(k) == v

    def test_denormalize_healthcheck(self):
        service_dict = {
            'image': 'test',
            'healthcheck': {
                'test': 'exit 1',
                'interval': '1m40s',
                'timeout': '30s',
                'retries': 5,
                'start_period': '2s90ms'
            }
        }
        processed_service = config.process_service(config.ServiceConfig(
            '.', 'test', 'test', service_dict
        ))
        denormalized_service = denormalize_service_dict(processed_service, VERSION)
        assert denormalized_service['healthcheck']['interval'] == '100s'
        assert denormalized_service['healthcheck']['timeout'] == '30s'
        assert denormalized_service['healthcheck']['start_period'] == '2090ms'

    def test_denormalize_image_has_digest(self):
        service_dict = {
            'image': 'busybox'
        }
        image_digest = 'busybox@sha256:abcde'

        assert denormalize_service_dict(service_dict, VERSION, image_digest) == {
            'image': 'busybox@sha256:abcde'
        }

    def test_denormalize_image_no_digest(self):
        service_dict = {
            'image': 'busybox'
        }

        assert denormalize_service_dict(service_dict, VERSION) == {
            'image': 'busybox'
        }

    def test_serialize_secrets(self):
        service_dict = {
            'image': 'example/web',
            'secrets': [
                {'source': 'one'},
                {
                    'source': 'source',
                    'target': 'target',
                    'uid': '100',
                    'gid': '200',
                    'mode': 0o777,
                }
            ]
        }
        secrets_dict = {
            'one': {'file': '/one.txt'},
            'source': {'file': '/source.pem'},
            'two': {'external': True},
        }
        config_dict = config.load(build_config_details({
            'version': '3.1',
            'services': {'web': service_dict},
            'secrets': secrets_dict
        }))

        serialized_config = yaml.safe_load(serialize_config(config_dict))
        serialized_service = serialized_config['services']['web']
        assert secret_sort(serialized_service['secrets']) == secret_sort(service_dict['secrets'])
        assert 'secrets' in serialized_config
        assert serialized_config['secrets']['two'] == {'external': True, 'name': 'two'}

    def test_serialize_ports(self):
        config_dict = config.Config(config_version=VERSION, version=VERSION, services=[
            {
                'ports': [types.ServicePort('80', '8080', None, None, None)],
                'image': 'alpine',
                'name': 'web'
            }
        ], volumes={}, networks={}, secrets={}, configs={})

        serialized_config = yaml.safe_load(serialize_config(config_dict))
        assert [{'published': 8080, 'target': 80}] == serialized_config['services']['web']['ports']

    def test_serialize_ports_v1(self):
        config_dict = config.Config(config_version=V1, version=V1, services=[
            {
                'ports': [types.ServicePort('80', '8080', None, None, None)],
                'image': 'alpine',
                'name': 'web'
            }
        ], volumes={}, networks={}, secrets={}, configs={})

        serialized_config = yaml.safe_load(serialize_config(config_dict))
        assert ['8080:80/tcp'] == serialized_config['services']['web']['ports']

    def test_serialize_ports_with_ext_ip(self):
        config_dict = config.Config(config_version=VERSION, version=VERSION, services=[
            {
                'ports': [types.ServicePort('80', '8080', None, None, '127.0.0.1')],
                'image': 'alpine',
                'name': 'web'
            }
        ], volumes={}, networks={}, secrets={}, configs={})

        serialized_config = yaml.safe_load(serialize_config(config_dict))
        assert '127.0.0.1:8080:80/tcp' in serialized_config['services']['web']['ports']

    def test_serialize_configs(self):
        service_dict = {
            'image': 'example/web',
            'configs': [
                {'source': 'one'},
                {
                    'source': 'source',
                    'target': 'target',
                    'uid': '100',
                    'gid': '200',
                    'mode': 0o777,
                }
            ]
        }
        configs_dict = {
            'one': {'file': '/one.txt'},
            'source': {'file': '/source.pem'},
            'two': {'external': True},
        }
        config_dict = config.load(build_config_details({
            'version': '3.3',
            'services': {'web': service_dict},
            'configs': configs_dict
        }))

        serialized_config = yaml.safe_load(serialize_config(config_dict))
        serialized_service = serialized_config['services']['web']
        assert secret_sort(serialized_service['configs']) == secret_sort(service_dict['configs'])
        assert 'configs' in serialized_config
        assert serialized_config['configs']['two'] == {'external': True, 'name': 'two'}

    def test_serialize_bool_string(self):
        cfg = {
            'version': '2.2',
            'services': {
                'web': {
                    'image': 'example/web',
                    'command': 'true',
                    'environment': {'FOO': 'Y', 'BAR': 'on'}
                }
            }
        }
        config_dict = config.load(build_config_details(cfg))

        serialized_config = serialize_config(config_dict)
        assert 'command: "true"\n' in serialized_config
        assert 'FOO: "Y"\n' in serialized_config
        assert 'BAR: "on"\n' in serialized_config

    def test_serialize_escape_dollar_sign(self):
        cfg = {
            'version': '2.2',
            'services': {
                'web': {
                    'image': 'busybox',
                    'command': 'echo $$FOO',
                    'environment': {
                        'CURRENCY': '$$'
                    },
                    'entrypoint': ['$$SHELL', '-c'],
                }
            }
        }
        config_dict = config.load(build_config_details(cfg))

        serialized_config = yaml.safe_load(serialize_config(config_dict))
        serialized_service = serialized_config['services']['web']
        assert serialized_service['environment']['CURRENCY'] == '$$'
        assert serialized_service['command'] == 'echo $$FOO'
        assert serialized_service['entrypoint'][0] == '$$SHELL'

    def test_serialize_escape_dont_interpolate(self):
        cfg = {
            'version': '2.2',
            'services': {
                'web': {
                    'image': 'busybox',
                    'command': 'echo $FOO',
                    'environment': {
                        'CURRENCY': '$'
                    },
                    'env_file': ['tests/fixtures/env/three.env'],
                    'entrypoint': ['$SHELL', '-c'],
                }
            }
        }
        config_dict = config.load(build_config_details(cfg, working_dir='.'), interpolate=False)

        serialized_config = yaml.safe_load(serialize_config(config_dict, escape_dollar=False))
        serialized_service = serialized_config['services']['web']
        assert serialized_service['environment']['CURRENCY'] == '$'
        # Values coming from env_files are not allowed to have variables
        assert serialized_service['environment']['FOO'] == 'NO $$ENV VAR'
        assert serialized_service['environment']['DOO'] == 'NO $${ENV} VAR'
        assert serialized_service['command'] == 'echo $FOO'
        assert serialized_service['entrypoint'][0] == '$SHELL'

    def test_serialize_unicode_values(self):
        cfg = {
            'version': '2.3',
            'services': {
                'web': {
                    'image': 'busybox',
                    'command': 'echo 十六夜　咲夜'
                }
            }
        }

        config_dict = config.load(build_config_details(cfg))

        serialized_config = yaml.safe_load(serialize_config(config_dict))
        serialized_service = serialized_config['services']['web']
        assert serialized_service['command'] == 'echo 十六夜　咲夜'

    def test_serialize_external_false(self):
        cfg = {
            'version': '3.4',
            'volumes': {
                'test': {
                    'name': 'test-false',
                    'external': False
                }
            }
        }

        config_dict = config.load(build_config_details(cfg))
        serialized_config = yaml.safe_load(serialize_config(config_dict))
        serialized_volume = serialized_config['volumes']['test']
        assert serialized_volume['external'] is False
