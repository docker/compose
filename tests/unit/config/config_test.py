# encoding: utf-8
from __future__ import print_function

import os
import shutil
import tempfile
from operator import itemgetter

import py
import pytest

from compose.config import config
from compose.config.config import resolve_environment
from compose.config.errors import ConfigurationError
from compose.config.types import VolumeSpec
from compose.const import IS_WINDOWS_PLATFORM
from tests import mock
from tests import unittest


def make_service_dict(name, service_dict, working_dir, filename=None):
    """
    Test helper function to construct a ServiceExtendsResolver
    """
    resolver = config.ServiceExtendsResolver(config.ServiceConfig(
        working_dir=working_dir,
        filename=filename,
        name=name,
        config=service_dict))
    return config.process_service(resolver.run())


def service_sort(services):
    return sorted(services, key=itemgetter('name'))


def build_config_details(contents, working_dir='working_dir', filename='filename.yml'):
    return config.ConfigDetails(
        working_dir,
        [config.ConfigFile(filename, contents)])


class ConfigTest(unittest.TestCase):
    def test_load(self):
        service_dicts = config.load(
            build_config_details(
                {
                    'foo': {'image': 'busybox'},
                    'bar': {'image': 'busybox', 'environment': ['FOO=1']},
                },
                'tests/fixtures/extends',
                'common.yml'
            )
        )

        self.assertEqual(
            service_sort(service_dicts),
            service_sort([
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
        )

    def test_load_throws_error_when_not_dict(self):
        with self.assertRaises(ConfigurationError):
            config.load(
                build_config_details(
                    {'web': 'busybox:latest'},
                    'working_dir',
                    'filename.yml'
                )
            )

    def test_load_config_invalid_service_names(self):
        for invalid_name in ['?not?allowed', ' ', '', '!', '/', '\xe2']:
            with pytest.raises(ConfigurationError) as exc:
                config.load(build_config_details(
                    {invalid_name: {'image': 'busybox'}},
                    'working_dir',
                    'filename.yml'))
            assert 'Invalid service name \'%s\'' % invalid_name in exc.exconly()

    def test_load_with_invalid_field_name(self):
        config_details = build_config_details(
            {'web': {'image': 'busybox', 'name': 'bogus'}},
            'working_dir',
            'filename.yml')
        with pytest.raises(ConfigurationError) as exc:
            config.load(config_details)
        error_msg = "Unsupported config option for 'web' service: 'name'"
        assert error_msg in exc.exconly()
        assert "Validation failed in file 'filename.yml'" in exc.exconly()

    def test_load_invalid_service_definition(self):
        config_details = build_config_details(
            {'web': 'wrong'},
            'working_dir',
            'filename.yml')
        with pytest.raises(ConfigurationError) as exc:
            config.load(config_details)
        error_msg = "service 'web' doesn't have any configuration options"
        assert error_msg in exc.exconly()

    def test_config_integer_service_name_raise_validation_error(self):
        expected_error_msg = ("In file 'filename.yml' service name: 1 needs to "
                              "be a string, eg '1'")
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {1: {'image': 'busybox'}},
                    'working_dir',
                    'filename.yml'
                )
            )

    @pytest.mark.xfail(IS_WINDOWS_PLATFORM, reason='paths use slash')
    def test_load_with_multiple_files(self):
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

        service_dicts = config.load(details)
        expected = [
            {
                'name': 'web',
                'build': '/',
                'links': ['db'],
                'volumes': [VolumeSpec.parse('/home/user/project:/code')],
            },
            {
                'name': 'db',
                'image': 'example/db',
            },
        ]
        self.assertEqual(service_sort(service_dicts), service_sort(expected))

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

    def test_load_with_multiple_files_and_empty_base(self):
        base_file = config.ConfigFile('base.yml', None)
        override_file = config.ConfigFile(
            'override.yml',
            {'web': {'image': 'example/web'}})
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

        tmpdir = py.test.ensuretemp('config_test')
        self.addCleanup(tmpdir.remove)
        tmpdir.join('common.yml').write("""
            base:
              labels: ['label=one']
        """)
        with tmpdir.as_cwd():
            service_dicts = config.load(details)

        expected = [
            {
                'name': 'web',
                'image': 'example/web',
                'volumes': [VolumeSpec.parse('/home/user/project:/code')],
                'labels': {'label': 'one'},
            },
        ]
        self.assertEqual(service_sort(service_dicts), service_sort(expected))

    def test_load_with_multiple_files_and_invalid_override(self):
        base_file = config.ConfigFile(
            'base.yaml',
            {'web': {'image': 'example/web'}})
        override_file = config.ConfigFile(
            'override.yaml',
            {'bogus': 'thing'})
        details = config.ConfigDetails('.', [base_file, override_file])

        with pytest.raises(ConfigurationError) as exc:
            config.load(details)
        assert "service 'bogus' doesn't have any configuration" in exc.exconly()
        assert "In file 'override.yaml'" in exc.exconly()

    def test_load_sorts_in_dependency_order(self):
        config_details = build_config_details({
            'web': {
                'image': 'busybox:latest',
                'links': ['db'],
            },
            'db': {
                'image': 'busybox:latest',
                'volumes_from': ['volume:ro']
            },
            'volume': {
                'image': 'busybox:latest',
                'volumes': ['/tmp'],
            }
        })
        services = config.load(config_details)

        assert services[0]['name'] == 'volume'
        assert services[1]['name'] == 'db'
        assert services[2]['name'] == 'web'

    def test_config_valid_service_names(self):
        for valid_name in ['_', '-', '.__.', '_what-up.', 'what_.up----', 'whatup']:
            services = config.load(
                build_config_details(
                    {valid_name: {'image': 'busybox'}},
                    'tests/fixtures/extends',
                    'common.yml'))
            assert services[0]['name'] == valid_name

    def test_config_hint(self):
        expected_error_msg = "(did you mean 'privileged'?)"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {
                        'foo': {'image': 'busybox', 'privilige': 'something'},
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_invalid_config_build_and_image_specified(self):
        expected_error_msg = "Service 'foo' has both an image and build path specified."
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {
                        'foo': {'image': 'busybox', 'build': '.'},
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_invalid_config_type_should_be_an_array(self):
        expected_error_msg = "Service 'foo' configuration key 'links' contains an invalid type, it should be an array"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {
                        'foo': {'image': 'busybox', 'links': 'an_link'},
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_invalid_config_not_a_dictionary(self):
        expected_error_msg = ("Top level object in 'filename.yml' needs to be "
                              "an object.")
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    ['foo', 'lol'],
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_invalid_config_not_unique_items(self):
        expected_error_msg = "has non-unique elements"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {
                        'web': {'build': '.', 'devices': ['/dev/foo:/dev/foo', '/dev/foo:/dev/foo']}
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_invalid_list_of_strings_format(self):
        expected_error_msg = "Service 'web' configuration key 'command' contains 1"
        expected_error_msg += ", which is an invalid type, it should be a string"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {
                        'web': {'build': '.', 'command': [1]}
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_config_image_and_dockerfile_raise_validation_error(self):
        expected_error_msg = "Service 'web' has both an image and alternate Dockerfile."
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {'web': {'image': 'busybox', 'dockerfile': 'Dockerfile.alt'}},
                    'working_dir',
                    'filename.yml'
                )
            )

    def test_config_extra_hosts_string_raises_validation_error(self):
        expected_error_msg = "Service 'web' configuration key 'extra_hosts' contains an invalid type"

        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {'web': {
                        'image': 'busybox',
                        'extra_hosts': 'somehost:162.242.195.82'
                    }},
                    'working_dir',
                    'filename.yml'
                )
            )

    def test_config_extra_hosts_list_of_dicts_validation_error(self):
        expected_error_msg = "key 'extra_hosts' contains {'somehost': '162.242.195.82'}, which is an invalid type, it should be a string"

        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {'web': {
                        'image': 'busybox',
                        'extra_hosts': [
                            {'somehost': '162.242.195.82'},
                            {'otherhost': '50.31.209.229'}
                        ]
                    }},
                    'working_dir',
                    'filename.yml'
                )
            )

    def test_config_ulimits_invalid_keys_validation_error(self):
        expected = ("Service 'web' configuration key 'ulimits' 'nofile' contains "
                    "unsupported option: 'not_soft_or_hard'")

        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details(
                {
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
                },
                'working_dir',
                'filename.yml'))
        assert expected in exc.exconly()

    def test_config_ulimits_required_keys_validation_error(self):

        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details(
                {
                    'web': {
                        'image': 'busybox',
                        'ulimits': {'nofile': {"soft": 10000}}
                    }
                },
                'working_dir',
                'filename.yml'))
        assert "Service 'web' configuration key 'ulimits' 'nofile'" in exc.exconly()
        assert "'hard' is a required property" in exc.exconly()

    def test_config_ulimits_soft_greater_than_hard_error(self):
        expected = "cannot contain a 'soft' value higher than 'hard' value"

        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details(
                {
                    'web': {
                        'image': 'busybox',
                        'ulimits': {
                            'nofile': {"soft": 10000, "hard": 1000}
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
                    {'web': {
                        'image': 'busybox',
                        'expose': expose
                    }},
                    'working_dir',
                    'filename.yml'
                )
            )
            self.assertEqual(service[0]['expose'], expose)

    def test_valid_config_oneof_string_or_list(self):
        entrypoint_values = [["sh"], "sh"]
        for entrypoint in entrypoint_values:
            service = config.load(
                build_config_details(
                    {'web': {
                        'image': 'busybox',
                        'entrypoint': entrypoint
                    }},
                    'working_dir',
                    'filename.yml'
                )
            )
            self.assertEqual(service[0]['entrypoint'], entrypoint)

    @mock.patch('compose.config.validation.log')
    def test_logs_warning_for_boolean_in_environment(self, mock_logging):
        expected_warning_msg = "There is a boolean value in the 'environment' key."
        config.load(
            build_config_details(
                {'web': {
                    'image': 'busybox',
                    'environment': {'SHOW_STUFF': True}
                }},
                'working_dir',
                'filename.yml'
            )
        )

        self.assertTrue(mock_logging.warn.called)
        self.assertTrue(expected_warning_msg in mock_logging.warn.call_args[0][0])

    def test_config_valid_environment_dict_key_contains_dashes(self):
        services = config.load(
            build_config_details(
                {'web': {
                    'image': 'busybox',
                    'environment': {'SPRING_JPA_HIBERNATE_DDL-AUTO': 'none'}
                }},
                'working_dir',
                'filename.yml'
            )
        )
        self.assertEqual(services[0]['environment']['SPRING_JPA_HIBERNATE_DDL-AUTO'], 'none')

    def test_load_yaml_with_yaml_error(self):
        tmpdir = py.test.ensuretemp('invalid_yaml_test')
        self.addCleanup(tmpdir.remove)
        invalid_yaml_file = tmpdir.join('docker-compose.yml')
        invalid_yaml_file.write("""
            web:
              this is bogus: ok: what
        """)
        with pytest.raises(ConfigurationError) as exc:
            config.load_yaml(str(invalid_yaml_file))

        assert 'line 3, column 32' in exc.exconly()

    def test_validate_extra_hosts_invalid(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details({
                'web': {
                    'image': 'alpine',
                    'extra_hosts': "www.example.com: 192.168.0.17",
                }
            }))
        assert "'extra_hosts' contains an invalid type" in exc.exconly()

    def test_validate_extra_hosts_invalid_list(self):
        with pytest.raises(ConfigurationError) as exc:
            config.load(build_config_details({
                'web': {
                    'image': 'alpine',
                    'extra_hosts': [
                        {'www.example.com': '192.168.0.17'},
                        {'api.example.com': '192.168.0.18'}
                    ],
                }
            }))
        assert "which is an invalid type" in exc.exconly()

    def test_normalize_dns_options(self):
        actual = config.load(build_config_details({
            'web': {
                'image': 'alpine',
                'dns': '8.8.8.8',
                'dns_search': 'domain.local',
            }
        }))
        assert actual == [
            {
                'name': 'web',
                'image': 'alpine',
                'dns': ['8.8.8.8'],
                'dns_search': ['domain.local'],
            }
        ]


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
        ["8000-8001:8000"],
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
            build_config_details(
                {'web': dict(image='busybox', **cfg)},
                'working_dir',
                'filename.yml'
            )
        )


class InterpolationTest(unittest.TestCase):
    @mock.patch.dict(os.environ)
    def test_config_file_with_environment_variable(self):
        os.environ.update(
            IMAGE="busybox",
            HOST_PORT="80",
            LABEL_VALUE="myvalue",
        )

        service_dicts = config.load(
            config.find('tests/fixtures/environment-interpolation', None),
        )

        self.assertEqual(service_dicts, [
            {
                'name': 'web',
                'image': 'busybox',
                'ports': ['80:8000'],
                'labels': {'mylabel': 'myvalue'},
                'hostname': 'host-',
                'command': '${ESCAPED}',
            }
        ])

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

        with mock.patch('compose.config.interpolation.log') as log:
            config.load(config_details)

            self.assertEqual(2, log.warn.call_count)
            warnings = sorted(args[0][0] for args in log.warn.call_args_list)
            self.assertIn('BAR', warnings[0])
            self.assertIn('FOO', warnings[1])

    @mock.patch.dict(os.environ)
    def test_invalid_interpolation(self):
        with self.assertRaises(config.ConfigurationError) as cm:
            config.load(
                build_config_details(
                    {'web': {'image': '${'}},
                    'working_dir',
                    'filename.yml'
                )
            )

        self.assertIn('Invalid', cm.exception.msg)
        self.assertIn('for "image" option', cm.exception.msg)
        self.assertIn('in service "web"', cm.exception.msg)
        self.assertIn('"${"', cm.exception.msg)

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
        )[0]
        self.assertEquals(service_dict['environment']['POSTGRES_PASSWORD'], '')


class VolumeConfigTest(unittest.TestCase):
    def test_no_binding(self):
        d = make_service_dict('foo', {'build': '.', 'volumes': ['/data']}, working_dir='.')
        self.assertEqual(d['volumes'], ['/data'])

    @mock.patch.dict(os.environ)
    def test_volume_binding_with_environment_variable(self):
        os.environ['VOLUME_PATH'] = '/host/path'
        d = config.load(build_config_details(
            {'foo': {'build': '.', 'volumes': ['${VOLUME_PATH}:/container/path']}},
            '.',
        ))[0]
        self.assertEqual(d['volumes'], [VolumeSpec.parse('/host/path:/container/path')])

    @pytest.mark.skipif(IS_WINDOWS_PLATFORM, reason='posix paths')
    @mock.patch.dict(os.environ)
    def test_volume_binding_with_home(self):
        os.environ['HOME'] = '/home/user'
        d = make_service_dict('foo', {'build': '.', 'volumes': ['~:/container/path']}, working_dir='.')
        self.assertEqual(d['volumes'], ['/home/user:/container/path'])

    def test_name_does_not_expand(self):
        d = make_service_dict('foo', {'build': '.', 'volumes': ['mydatavolume:/data']}, working_dir='.')
        self.assertEqual(d['volumes'], ['mydatavolume:/data'])

    def test_absolute_posix_path_does_not_expand(self):
        d = make_service_dict('foo', {'build': '.', 'volumes': ['/var/lib/data:/data']}, working_dir='.')
        self.assertEqual(d['volumes'], ['/var/lib/data:/data'])

    def test_absolute_windows_path_does_not_expand(self):
        d = make_service_dict('foo', {'build': '.', 'volumes': ['c:\\data:/data']}, working_dir='.')
        self.assertEqual(d['volumes'], ['c:\\data:/data'])

    @pytest.mark.skipif(IS_WINDOWS_PLATFORM, reason='posix paths')
    def test_relative_path_does_expand_posix(self):
        d = make_service_dict('foo', {'build': '.', 'volumes': ['./data:/data']}, working_dir='/home/me/myproject')
        self.assertEqual(d['volumes'], ['/home/me/myproject/data:/data'])

        d = make_service_dict('foo', {'build': '.', 'volumes': ['.:/data']}, working_dir='/home/me/myproject')
        self.assertEqual(d['volumes'], ['/home/me/myproject:/data'])

        d = make_service_dict('foo', {'build': '.', 'volumes': ['../otherproject:/data']}, working_dir='/home/me/myproject')
        self.assertEqual(d['volumes'], ['/home/me/otherproject:/data'])

    @pytest.mark.skipif(not IS_WINDOWS_PLATFORM, reason='windows paths')
    def test_relative_path_does_expand_windows(self):
        d = make_service_dict('foo', {'build': '.', 'volumes': ['./data:/data']}, working_dir='c:\\Users\\me\\myproject')
        self.assertEqual(d['volumes'], ['c:\\Users\\me\\myproject\\data:/data'])

        d = make_service_dict('foo', {'build': '.', 'volumes': ['.:/data']}, working_dir='c:\\Users\\me\\myproject')
        self.assertEqual(d['volumes'], ['c:\\Users\\me\\myproject:/data'])

        d = make_service_dict('foo', {'build': '.', 'volumes': ['../otherproject:/data']}, working_dir='c:\\Users\\me\\myproject')
        self.assertEqual(d['volumes'], ['c:\\Users\\me\\otherproject:/data'])

    @mock.patch.dict(os.environ)
    def test_home_directory_with_driver_does_not_expand(self):
        os.environ['NAME'] = 'surprise!'
        d = make_service_dict('foo', {
            'build': '.',
            'volumes': ['~:/data'],
            'volume_driver': 'foodriver',
        }, working_dir='.')
        self.assertEqual(d['volumes'], ['~:/data'])

    def test_volume_path_with_non_ascii_directory(self):
        volume = u'/Füü/data:/data'
        container_path = config.resolve_volume_path(".", volume)
        self.assertEqual(container_path, volume)


class MergePathMappingTest(object):
    def config_name(self):
        return ""

    def test_empty(self):
        service_dict = config.merge_service_dicts({}, {})
        self.assertNotIn(self.config_name(), service_dict)

    def test_no_override(self):
        service_dict = config.merge_service_dicts(
            {self.config_name(): ['/foo:/code', '/data']},
            {},
        )
        self.assertEqual(set(service_dict[self.config_name()]), set(['/foo:/code', '/data']))

    def test_no_base(self):
        service_dict = config.merge_service_dicts(
            {},
            {self.config_name(): ['/bar:/code']},
        )
        self.assertEqual(set(service_dict[self.config_name()]), set(['/bar:/code']))

    def test_override_explicit_path(self):
        service_dict = config.merge_service_dicts(
            {self.config_name(): ['/foo:/code', '/data']},
            {self.config_name(): ['/bar:/code']},
        )
        self.assertEqual(set(service_dict[self.config_name()]), set(['/bar:/code', '/data']))

    def test_add_explicit_path(self):
        service_dict = config.merge_service_dicts(
            {self.config_name(): ['/foo:/code', '/data']},
            {self.config_name(): ['/bar:/code', '/quux:/data']},
        )
        self.assertEqual(set(service_dict[self.config_name()]), set(['/bar:/code', '/quux:/data']))

    def test_remove_explicit_path(self):
        service_dict = config.merge_service_dicts(
            {self.config_name(): ['/foo:/code', '/quux:/data']},
            {self.config_name(): ['/bar:/code', '/data']},
        )
        self.assertEqual(set(service_dict[self.config_name()]), set(['/bar:/code', '/data']))


class MergeVolumesTest(unittest.TestCase, MergePathMappingTest):
    def config_name(self):
        return 'volumes'


class MergeDevicesTest(unittest.TestCase, MergePathMappingTest):
    def config_name(self):
        return 'devices'


class BuildOrImageMergeTest(unittest.TestCase):
    def test_merge_build_or_image_no_override(self):
        self.assertEqual(
            config.merge_service_dicts({'build': '.'}, {}),
            {'build': '.'},
        )

        self.assertEqual(
            config.merge_service_dicts({'image': 'redis'}, {}),
            {'image': 'redis'},
        )

    def test_merge_build_or_image_override_with_same(self):
        self.assertEqual(
            config.merge_service_dicts({'build': '.'}, {'build': './web'}),
            {'build': './web'},
        )

        self.assertEqual(
            config.merge_service_dicts({'image': 'redis'}, {'image': 'postgres'}),
            {'image': 'postgres'},
        )

    def test_merge_build_or_image_override_with_other(self):
        self.assertEqual(
            config.merge_service_dicts({'build': '.'}, {'image': 'redis'}),
            {'image': 'redis'}
        )

        self.assertEqual(
            config.merge_service_dicts({'image': 'redis'}, {'build': '.'}),
            {'build': '.'}
        )


class MergeListsTest(unittest.TestCase):
    def test_empty(self):
        service_dict = config.merge_service_dicts({}, {})
        self.assertNotIn('ports', service_dict)

    def test_no_override(self):
        service_dict = config.merge_service_dicts(
            {'ports': ['10:8000', '9000']},
            {},
        )
        self.assertEqual(set(service_dict['ports']), set(['10:8000', '9000']))

    def test_no_base(self):
        service_dict = config.merge_service_dicts(
            {},
            {'ports': ['10:8000', '9000']},
        )
        self.assertEqual(set(service_dict['ports']), set(['10:8000', '9000']))

    def test_add_item(self):
        service_dict = config.merge_service_dicts(
            {'ports': ['10:8000', '9000']},
            {'ports': ['20:8000']},
        )
        self.assertEqual(set(service_dict['ports']), set(['10:8000', '9000', '20:8000']))


class MergeStringsOrListsTest(unittest.TestCase):
    def test_no_override(self):
        service_dict = config.merge_service_dicts(
            {'dns': '8.8.8.8'},
            {},
        )
        self.assertEqual(set(service_dict['dns']), set(['8.8.8.8']))

    def test_no_base(self):
        service_dict = config.merge_service_dicts(
            {},
            {'dns': '8.8.8.8'},
        )
        self.assertEqual(set(service_dict['dns']), set(['8.8.8.8']))

    def test_add_string(self):
        service_dict = config.merge_service_dicts(
            {'dns': ['8.8.8.8']},
            {'dns': '9.9.9.9'},
        )
        self.assertEqual(set(service_dict['dns']), set(['8.8.8.8', '9.9.9.9']))

    def test_add_list(self):
        service_dict = config.merge_service_dicts(
            {'dns': '8.8.8.8'},
            {'dns': ['9.9.9.9']},
        )
        self.assertEqual(set(service_dict['dns']), set(['8.8.8.8', '9.9.9.9']))


class MergeLabelsTest(unittest.TestCase):
    def test_empty(self):
        service_dict = config.merge_service_dicts({}, {})
        self.assertNotIn('labels', service_dict)

    def test_no_override(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'build': '.', 'labels': ['foo=1', 'bar']}, 'tests/'),
            make_service_dict('foo', {'build': '.'}, 'tests/'),
        )
        self.assertEqual(service_dict['labels'], {'foo': '1', 'bar': ''})

    def test_no_base(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'build': '.'}, 'tests/'),
            make_service_dict('foo', {'build': '.', 'labels': ['foo=2']}, 'tests/'),
        )
        self.assertEqual(service_dict['labels'], {'foo': '2'})

    def test_override_explicit_value(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'build': '.', 'labels': ['foo=1', 'bar']}, 'tests/'),
            make_service_dict('foo', {'build': '.', 'labels': ['foo=2']}, 'tests/'),
        )
        self.assertEqual(service_dict['labels'], {'foo': '2', 'bar': ''})

    def test_add_explicit_value(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'build': '.', 'labels': ['foo=1', 'bar']}, 'tests/'),
            make_service_dict('foo', {'build': '.', 'labels': ['bar=2']}, 'tests/'),
        )
        self.assertEqual(service_dict['labels'], {'foo': '1', 'bar': '2'})

    def test_remove_explicit_value(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'build': '.', 'labels': ['foo=1', 'bar=2']}, 'tests/'),
            make_service_dict('foo', {'build': '.', 'labels': ['bar']}, 'tests/'),
        )
        self.assertEqual(service_dict['labels'], {'foo': '1', 'bar': ''})


class MemoryOptionsTest(unittest.TestCase):
    def test_validation_fails_with_just_memswap_limit(self):
        """
        When you set a 'memswap_limit' it is invalid config unless you also set
        a mem_limit
        """
        expected_error_msg = (
            "Service 'foo' configuration key 'memswap_limit' is invalid: when "
            "defining 'memswap_limit' you must set 'mem_limit' as well"
        )
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {
                        'foo': {'image': 'busybox', 'memswap_limit': 2000000},
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_validation_with_correct_memswap_values(self):
        service_dict = config.load(
            build_config_details(
                {'foo': {'image': 'busybox', 'mem_limit': 1000000, 'memswap_limit': 2000000}},
                'tests/fixtures/extends',
                'common.yml'
            )
        )
        self.assertEqual(service_dict[0]['memswap_limit'], 2000000)

    def test_memswap_can_be_a_string(self):
        service_dict = config.load(
            build_config_details(
                {'foo': {'image': 'busybox', 'mem_limit': "1G", 'memswap_limit': "512M"}},
                'tests/fixtures/extends',
                'common.yml'
            )
        )
        self.assertEqual(service_dict[0]['memswap_limit'], "512M")


class EnvTest(unittest.TestCase):
    def test_parse_environment_as_list(self):
        environment = [
            'NORMAL=F1',
            'CONTAINS_EQUALS=F=2',
            'TRAILING_EQUALS=',
        ]
        self.assertEqual(
            config.parse_environment(environment),
            {'NORMAL': 'F1', 'CONTAINS_EQUALS': 'F=2', 'TRAILING_EQUALS': ''},
        )

    def test_parse_environment_as_dict(self):
        environment = {
            'NORMAL': 'F1',
            'CONTAINS_EQUALS': 'F=2',
            'TRAILING_EQUALS': None,
        }
        self.assertEqual(config.parse_environment(environment), environment)

    def test_parse_environment_invalid(self):
        with self.assertRaises(ConfigurationError):
            config.parse_environment('a=b')

    def test_parse_environment_empty(self):
        self.assertEqual(config.parse_environment(None), {})

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
        self.assertEqual(
            resolve_environment(service_dict),
            {'FILE_DEF': 'F1', 'FILE_DEF_EMPTY': '', 'ENV_DEF': 'E3', 'NO_DEF': ''},
        )

    def test_resolve_environment_from_env_file(self):
        self.assertEqual(
            resolve_environment({'env_file': ['tests/fixtures/env/one.env']}),
            {'ONE': '2', 'TWO': '1', 'THREE': '3', 'FOO': 'bar'},
        )

    def test_resolve_environment_with_multiple_env_files(self):
        service_dict = {
            'env_file': [
                'tests/fixtures/env/one.env',
                'tests/fixtures/env/two.env'
            ]
        }
        self.assertEqual(
            resolve_environment(service_dict),
            {'ONE': '2', 'TWO': '1', 'THREE': '3', 'FOO': 'baz', 'DOO': 'dah'},
        )

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
        self.assertEqual(
            resolve_environment({'env_file': ['tests/fixtures/env/resolve.env']}),
            {
                'FILE_DEF': u'bär',
                'FILE_DEF_EMPTY': '',
                'ENV_DEF': 'E3',
                'NO_DEF': ''
            },
        )

    @pytest.mark.xfail(IS_WINDOWS_PLATFORM, reason='paths use slash')
    @mock.patch.dict(os.environ)
    def test_resolve_path(self):
        os.environ['HOSTENV'] = '/tmp'
        os.environ['CONTAINERENV'] = '/host/tmp'

        service_dict = config.load(
            build_config_details(
                {'foo': {'build': '.', 'volumes': ['$HOSTENV:$CONTAINERENV']}},
                "tests/fixtures/env",
            )
        )[0]
        self.assertEqual(
            set(service_dict['volumes']),
            set([VolumeSpec.parse('/tmp:/host/tmp')]))

        service_dict = config.load(
            build_config_details(
                {'foo': {'build': '.', 'volumes': ['/opt${HOSTENV}:/opt${CONTAINERENV}']}},
                "tests/fixtures/env",
            )
        )[0]
        self.assertEqual(
            set(service_dict['volumes']),
            set([VolumeSpec.parse('/opt/tmp:/opt/host/tmp')]))


def load_from_filename(filename):
    return config.load(config.find('.', [filename]))


class ExtendsTest(unittest.TestCase):
    def test_extends(self):
        service_dicts = load_from_filename('tests/fixtures/extends/docker-compose.yml')

        self.assertEqual(service_sort(service_dicts), service_sort([
            {
                'name': 'mydb',
                'image': 'busybox',
                'command': 'top',
            },
            {
                'name': 'myweb',
                'image': 'busybox',
                'command': 'top',
                'links': ['mydb:db'],
                'environment': {
                    "FOO": "1",
                    "BAR": "2",
                    "BAZ": "2",
                },
            }
        ]))

    def test_nested(self):
        service_dicts = load_from_filename('tests/fixtures/extends/nested.yml')

        self.assertEqual(service_dicts, [
            {
                'name': 'myweb',
                'image': 'busybox',
                'command': '/bin/true',
                'environment': {
                    "FOO": "2",
                    "BAR": "2",
                },
            },
        ])

    def test_self_referencing_file(self):
        """
        We specify a 'file' key that is the filename we're already in.
        """
        service_dicts = load_from_filename('tests/fixtures/extends/specify-file-as-self.yml')
        self.assertEqual(service_sort(service_dicts), service_sort([
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
        ]))

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
        self.assertEqual(path, expected)

    def test_extends_validation_empty_dictionary(self):
        with self.assertRaisesRegexp(ConfigurationError, 'service'):
            config.load(
                build_config_details(
                    {
                        'web': {'image': 'busybox', 'extends': {}},
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_extends_validation_missing_service_key(self):
        with self.assertRaisesRegexp(ConfigurationError, "'service' is a required property"):
            config.load(
                build_config_details(
                    {
                        'web': {'image': 'busybox', 'extends': {'file': 'common.yml'}},
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_extends_validation_invalid_key(self):
        expected_error_msg = (
            "Service 'web' configuration key 'extends' "
            "contains unsupported option: 'rogue_key'"
        )
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {
                        'web': {
                            'image': 'busybox',
                            'extends': {
                                'file': 'common.yml',
                                'service': 'web',
                                'rogue_key': 'is not allowed'
                            }
                        },
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_extends_validation_sub_property_key(self):
        expected_error_msg = (
            "Service 'web' configuration key 'extends' 'file' contains 1, "
            "which is an invalid type, it should be a string"
        )
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                build_config_details(
                    {
                        'web': {
                            'image': 'busybox',
                            'extends': {
                                'file': 1,
                                'service': 'web',
                            }
                        },
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_extends_validation_no_file_key_no_filename_set(self):
        dictionary = {'extends': {'service': 'web'}}

        def load_config():
            return make_service_dict('myweb', dictionary, working_dir='tests/fixtures/extends')

        self.assertRaisesRegexp(ConfigurationError, 'file', load_config)

    def test_extends_validation_valid_config(self):
        service = config.load(
            build_config_details(
                {
                    'web': {'image': 'busybox', 'extends': {'service': 'web', 'file': 'common.yml'}},
                },
                'tests/fixtures/extends',
                'common.yml'
            )
        )

        self.assertEquals(len(service), 1)
        self.assertIsInstance(service[0], dict)
        self.assertEquals(service[0]['command'], "/bin/true")

    def test_extended_service_with_invalid_config(self):
        expected_error_msg = "Service 'myweb' has neither an image nor a build path specified"

        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            load_from_filename('tests/fixtures/extends/service-with-invalid-schema.yml')

    def test_extended_service_with_valid_config(self):
        service = load_from_filename('tests/fixtures/extends/service-with-valid-composite-extends.yml')
        self.assertEquals(service[0]['command'], "top")

    def test_extends_file_defaults_to_self(self):
        """
        Test not specifying a file in our extends options that the
        config is valid and correctly extends from itself.
        """
        service_dicts = load_from_filename('tests/fixtures/extends/no-file-specified.yml')
        self.assertEqual(service_sort(service_dicts), service_sort([
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
        ]))

    def test_invalid_links_in_extended_service(self):
        expected_error_msg = "services with 'links' cannot be extended"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            load_from_filename('tests/fixtures/extends/invalid-links.yml')

    def test_invalid_volumes_from_in_extended_service(self):
        expected_error_msg = "services with 'volumes_from' cannot be extended"

        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            load_from_filename('tests/fixtures/extends/invalid-volumes.yml')

    def test_invalid_net_in_extended_service(self):
        expected_error_msg = "services with 'net: container' cannot be extended"

        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            load_from_filename('tests/fixtures/extends/invalid-net.yml')

    @mock.patch.dict(os.environ)
    def test_valid_interpolation_in_extended_service(self):
        os.environ.update(
            HOSTNAME_VALUE="penguin",
        )
        expected_interpolated_value = "host-penguin"

        service_dicts = load_from_filename('tests/fixtures/extends/valid-interpolation.yml')
        for service in service_dicts:
            self.assertTrue(service['hostname'], expected_interpolated_value)

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

        self.assertEqual(set(dicts[0]['volumes']), set(paths))

    def test_parent_build_path_dne(self):
        child = load_from_filename('tests/fixtures/extends/nonexistent-path-child.yml')

        self.assertEqual(child, [
            {
                'name': 'dnechild',
                'image': 'busybox',
                'command': '/bin/true',
                'environment': {
                    "FOO": "1",
                    "BAR": "2",
                },
            },
        ])

    def test_load_throws_error_when_base_service_does_not_exist(self):
        err_msg = r'''Cannot extend service 'foo' in .*: Service not found'''
        with self.assertRaisesRegexp(ConfigurationError, err_msg):
            load_from_filename('tests/fixtures/extends/nonexistent-service.yml')

    def test_partial_service_config_in_extends_is_still_valid(self):
        dicts = load_from_filename('tests/fixtures/extends/valid-common-config.yml')
        self.assertEqual(dicts[0]['environment'], {'FOO': '1'})

    def test_extended_service_with_verbose_and_shorthand_way(self):
        services = load_from_filename('tests/fixtures/extends/verbose-and-shorthand.yml')
        self.assertEqual(service_sort(services), service_sort([
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
        ]))

    def test_extends_with_environment_and_env_files(self):
        tmpdir = py.test.ensuretemp('test_extends_with_environment')
        self.addCleanup(tmpdir.remove)
        commondir = tmpdir.mkdir('common')
        commondir.join('base.yml').write("""
            app:
                image: 'example/app'
                env_file:
                    - 'envs'
                environment:
                    - SECRET
                    - TEST_ONE=common
                    - TEST_TWO=common
        """)
        tmpdir.join('docker-compose.yml').write("""
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
        commondir.join('envs').write("""
            COMMON_ENV_FILE
            TEST_ONE=common-env-file
            TEST_TWO=common-env-file
            TEST_THREE=common-env-file
            TEST_FOUR=common-env-file
        """)
        tmpdir.join('envs').write("""
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
        with mock.patch.dict(os.environ):
            os.environ['SECRET'] = 'secret'
            os.environ['THING'] = 'thing'
            os.environ['COMMON_ENV_FILE'] = 'secret'
            os.environ['TOP_ENV_FILE'] = 'secret'
            config = load_from_filename(str(tmpdir.join('docker-compose.yml')))

        assert config == expected


@pytest.mark.xfail(IS_WINDOWS_PLATFORM, reason='paths use slash')
class ExpandPathTest(unittest.TestCase):
    working_dir = '/home/user/somedir'

    def test_expand_path_normal(self):
        result = config.expand_path(self.working_dir, 'myfile')
        self.assertEqual(result, self.working_dir + '/' + 'myfile')

    def test_expand_path_absolute(self):
        abs_path = '/home/user/otherdir/somefile'
        result = config.expand_path(self.working_dir, abs_path)
        self.assertEqual(result, abs_path)

    def test_expand_path_with_tilde(self):
        test_path = '~/otherdir/somefile'
        with mock.patch.dict(os.environ):
            os.environ['HOME'] = user_path = '/home/user/'
            result = config.expand_path(self.working_dir, test_path)

        self.assertEqual(result, user_path + 'otherdir/somefile')


class VolumePathTest(unittest.TestCase):

    @pytest.mark.xfail((not IS_WINDOWS_PLATFORM), reason='does not have a drive')
    def test_split_path_mapping_with_windows_path(self):
        windows_volume_path = "c:\\Users\\msamblanet\\Documents\\anvil\\connect\\config:/opt/connect/config:ro"
        expected_mapping = (
            "/opt/connect/config:ro",
            "c:\\Users\\msamblanet\\Documents\\anvil\\connect\\config"
        )

        mapping = config.split_path_mapping(windows_volume_path)

        self.assertEqual(mapping, expected_mapping)


@pytest.mark.xfail(IS_WINDOWS_PLATFORM, reason='paths use slash')
class BuildPathTest(unittest.TestCase):
    def setUp(self):
        self.abs_context_path = os.path.join(os.getcwd(), 'tests/fixtures/build-ctx')

    def test_nonexistent_path(self):
        with self.assertRaises(ConfigurationError):
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
        self.assertEquals(service_dict['build'], self.abs_context_path)

    def test_absolute_path(self):
        service_dict = make_service_dict(
            'abspath',
            {'build': self.abs_context_path},
            working_dir='tests/fixtures/build-path'
        )
        self.assertEquals(service_dict['build'], self.abs_context_path)

    def test_from_file(self):
        service_dict = load_from_filename('tests/fixtures/build-path/docker-compose.yml')
        self.assertEquals(service_dict, [{'name': 'foo', 'build': self.abs_context_path}])

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
            }, '.', None))
            assert service_dict[0]['build'] == valid_url

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


class GetDefaultConfigFilesTestCase(unittest.TestCase):

    files = [
        'docker-compose.yml',
        'docker-compose.yaml',
        'fig.yml',
        'fig.yaml',
    ]

    def test_get_config_path_default_file_in_basedir(self):
        for index, filename in enumerate(self.files):
            self.assertEqual(
                filename,
                get_config_filename_for_files(self.files[index:]))
        with self.assertRaises(config.ComposeFileNotFound):
            get_config_filename_for_files([])

    def test_get_config_path_default_file_in_parent_dir(self):
        """Test with files placed in the subdir"""

        def get_config_in_subdir(files):
            return get_config_filename_for_files(files, subdir=True)

        for index, filename in enumerate(self.files):
            self.assertEqual(filename, get_config_in_subdir(self.files[index:]))
        with self.assertRaises(config.ComposeFileNotFound):
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
        filename, = config.get_default_config_files(base_dir)
        return os.path.basename(filename)
    finally:
        shutil.rmtree(project_dir)
