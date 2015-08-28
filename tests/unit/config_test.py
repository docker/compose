from __future__ import print_function

import os
import shutil
import tempfile
from operator import itemgetter

from .. import mock
from .. import unittest
from compose.config import config
from compose.config.errors import ConfigurationError


def make_service_dict(name, service_dict, working_dir):
    """
    Test helper function to construct a ServiceLoader
    """
    return config.ServiceLoader(working_dir=working_dir).make_service_dict(name, service_dict)


def service_sort(services):
    return sorted(services, key=itemgetter('name'))


class ConfigTest(unittest.TestCase):
    def test_load(self):
        service_dicts = config.load(
            config.ConfigDetails(
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
                config.ConfigDetails(
                    {'web': 'busybox:latest'},
                    'working_dir',
                    'filename.yml'
                )
            )

    def test_config_invalid_service_names(self):
        with self.assertRaises(ConfigurationError):
            for invalid_name in ['?not?allowed', ' ', '', '!', '/', '\xe2']:
                config.load(
                    config.ConfigDetails(
                        {invalid_name: {'image': 'busybox'}},
                        'working_dir',
                        'filename.yml'
                    )
                )

    def test_config_integer_service_name_raise_validation_error(self):
        expected_error_msg = "Service name: 1 needs to be a string, eg '1'"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                config.ConfigDetails(
                    {1: {'image': 'busybox'}},
                    'working_dir',
                    'filename.yml'
                )
            )

    def test_config_valid_service_names(self):
        for valid_name in ['_', '-', '.__.', '_what-up.', 'what_.up----', 'whatup']:
            config.load(
                config.ConfigDetails(
                    {valid_name: {'image': 'busybox'}},
                    'tests/fixtures/extends',
                    'common.yml'
                )
            )

    def test_config_invalid_ports_format_validation(self):
        expected_error_msg = "Service 'web' configuration key 'ports' contains an invalid type"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            for invalid_ports in [{"1": "8000"}, False, 0, "8000", 8000, ["8000", "8000"]]:
                config.load(
                    config.ConfigDetails(
                        {'web': {'image': 'busybox', 'ports': invalid_ports}},
                        'working_dir',
                        'filename.yml'
                    )
                )

    def test_config_valid_ports_format_validation(self):
        valid_ports = [["8000", "9000"], ["8000/8050"], ["8000"], [8000], ["49153-49154:3002-3003"]]
        for ports in valid_ports:
            config.load(
                config.ConfigDetails(
                    {'web': {'image': 'busybox', 'ports': ports}},
                    'working_dir',
                    'filename.yml'
                )
            )

    def test_config_hint(self):
        expected_error_msg = "(did you mean 'privileged'?)"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                config.ConfigDetails(
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
                config.ConfigDetails(
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
                config.ConfigDetails(
                    {
                        'foo': {'image': 'busybox', 'links': 'an_link'},
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_invalid_config_not_a_dictionary(self):
        expected_error_msg = "Top level object needs to be a dictionary."
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                config.ConfigDetails(
                    ['foo', 'lol'],
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_invalid_config_not_unique_items(self):
        expected_error_msg = "has non-unique elements"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                config.ConfigDetails(
                    {
                        'web': {'build': '.', 'devices': ['/dev/foo:/dev/foo', '/dev/foo:/dev/foo']}
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_invalid_list_of_strings_format(self):
        expected_error_msg = "'command' contains an invalid type, valid types are string or list of strings"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                config.ConfigDetails(
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
                config.ConfigDetails(
                    {'web': {'image': 'busybox', 'dockerfile': 'Dockerfile.alt'}},
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
        config_details = config.ConfigDetails(
            config={
                'web': {
                    'image': '${FOO}',
                    'command': '${BAR}',
                    'entrypoint': '${BAR}',
                },
            },
            working_dir='.',
            filename=None,
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
                config.ConfigDetails(
                    {'web': {'image': '${'}},
                    'working_dir',
                    'filename.yml'
                )
            )

        self.assertIn('Invalid', cm.exception.msg)
        self.assertIn('for "image" option', cm.exception.msg)
        self.assertIn('in service "web"', cm.exception.msg)
        self.assertIn('"${"', cm.exception.msg)

    @mock.patch.dict(os.environ)
    def test_volume_binding_with_environment_variable(self):
        os.environ['VOLUME_PATH'] = '/host/path'
        d = config.load(
            config.ConfigDetails(
                config={'foo': {'build': '.', 'volumes': ['${VOLUME_PATH}:/container/path']}},
                working_dir='.',
                filename=None,
            )
        )[0]
        self.assertEqual(d['volumes'], ['/host/path:/container/path'])

    @mock.patch.dict(os.environ)
    def test_volume_binding_with_home(self):
        os.environ['HOME'] = '/home/user'
        d = make_service_dict('foo', {'volumes': ['~:/container/path']}, working_dir='.')
        self.assertEqual(d['volumes'], ['/home/user:/container/path'])

    @mock.patch.dict(os.environ)
    def test_volume_binding_with_local_dir_name_raises_warning(self):
        def make_dict(**config):
            make_service_dict('foo', config, working_dir='.')

        with mock.patch('compose.config.config.log.warn') as warn:
            make_dict(volumes=['/container/path'])
            self.assertEqual(0, warn.call_count)

            make_dict(volumes=['/data:/container/path'])
            self.assertEqual(0, warn.call_count)

            make_dict(volumes=['.:/container/path'])
            self.assertEqual(0, warn.call_count)

            make_dict(volumes=['..:/container/path'])
            self.assertEqual(0, warn.call_count)

            make_dict(volumes=['./data:/container/path'])
            self.assertEqual(0, warn.call_count)

            make_dict(volumes=['../data:/container/path'])
            self.assertEqual(0, warn.call_count)

            make_dict(volumes=['.profile:/container/path'])
            self.assertEqual(0, warn.call_count)

            make_dict(volumes=['~:/container/path'])
            self.assertEqual(0, warn.call_count)

            make_dict(volumes=['~/data:/container/path'])
            self.assertEqual(0, warn.call_count)

            make_dict(volumes=['~tmp:/container/path'])
            self.assertEqual(0, warn.call_count)

            make_dict(volumes=['data:/container/path'], volume_driver='mydriver')
            self.assertEqual(0, warn.call_count)

            make_dict(volumes=['data:/container/path'])
            self.assertEqual(1, warn.call_count)
            warning = warn.call_args[0][0]
            self.assertIn('"data:/container/path"', warning)
            self.assertIn('"./data:/container/path"', warning)

    def test_named_volume_with_driver_does_not_expand(self):
        d = make_service_dict('foo', {
            'volumes': ['namedvolume:/data'],
            'volume_driver': 'foodriver',
        }, working_dir='.')
        self.assertEqual(d['volumes'], ['namedvolume:/data'])

    @mock.patch.dict(os.environ)
    def test_home_directory_with_driver_does_not_expand(self):
        os.environ['NAME'] = 'surprise!'
        d = make_service_dict('foo', {
            'volumes': ['~:/data'],
            'volume_driver': 'foodriver',
        }, working_dir='.')
        self.assertEqual(d['volumes'], ['~:/data'])


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
            make_service_dict('foo', {'labels': ['foo=1', 'bar']}, 'tests/'),
            make_service_dict('foo', {}, 'tests/'),
        )
        self.assertEqual(service_dict['labels'], {'foo': '1', 'bar': ''})

    def test_no_base(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {}, 'tests/'),
            make_service_dict('foo', {'labels': ['foo=2']}, 'tests/'),
        )
        self.assertEqual(service_dict['labels'], {'foo': '2'})

    def test_override_explicit_value(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'labels': ['foo=1', 'bar']}, 'tests/'),
            make_service_dict('foo', {'labels': ['foo=2']}, 'tests/'),
        )
        self.assertEqual(service_dict['labels'], {'foo': '2', 'bar': ''})

    def test_add_explicit_value(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'labels': ['foo=1', 'bar']}, 'tests/'),
            make_service_dict('foo', {'labels': ['bar=2']}, 'tests/'),
        )
        self.assertEqual(service_dict['labels'], {'foo': '1', 'bar': '2'})

    def test_remove_explicit_value(self):
        service_dict = config.merge_service_dicts(
            make_service_dict('foo', {'labels': ['foo=1', 'bar=2']}, 'tests/'),
            make_service_dict('foo', {'labels': ['bar']}, 'tests/'),
        )
        self.assertEqual(service_dict['labels'], {'foo': '1', 'bar': ''})


class MemoryOptionsTest(unittest.TestCase):
    def test_validation_fails_with_just_memswap_limit(self):
        """
        When you set a 'memswap_limit' it is invalid config unless you also set
        a mem_limit
        """
        expected_error_msg = (
            "Invalid 'memswap_limit' configuration for 'foo' service: when "
            "defining 'memswap_limit' you must set 'mem_limit' as well"
        )
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                config.ConfigDetails(
                    {
                        'foo': {'image': 'busybox', 'memswap_limit': 2000000},
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_validation_with_correct_memswap_values(self):
        service_dict = config.load(
            config.ConfigDetails(
                {'foo': {'image': 'busybox', 'mem_limit': 1000000, 'memswap_limit': 2000000}},
                'tests/fixtures/extends',
                'common.yml'
            )
        )
        self.assertEqual(service_dict[0]['memswap_limit'], 2000000)

    def test_memswap_can_be_a_string(self):
        service_dict = config.load(
            config.ConfigDetails(
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

        service_dict = make_service_dict(
            'foo', {
                'environment': {
                    'FILE_DEF': 'F1',
                    'FILE_DEF_EMPTY': '',
                    'ENV_DEF': None,
                    'NO_DEF': None
                },
            },
            'tests/'
        )

        self.assertEqual(
            service_dict['environment'],
            {'FILE_DEF': 'F1', 'FILE_DEF_EMPTY': '', 'ENV_DEF': 'E3', 'NO_DEF': ''},
        )

    def test_env_from_file(self):
        service_dict = make_service_dict(
            'foo',
            {'env_file': 'one.env'},
            'tests/fixtures/env',
        )
        self.assertEqual(
            service_dict['environment'],
            {'ONE': '2', 'TWO': '1', 'THREE': '3', 'FOO': 'bar'},
        )

    def test_env_from_multiple_files(self):
        service_dict = make_service_dict(
            'foo',
            {'env_file': ['one.env', 'two.env']},
            'tests/fixtures/env',
        )
        self.assertEqual(
            service_dict['environment'],
            {'ONE': '2', 'TWO': '1', 'THREE': '3', 'FOO': 'baz', 'DOO': 'dah'},
        )

    def test_env_nonexistent_file(self):
        options = {'env_file': 'nonexistent.env'}
        self.assertRaises(
            ConfigurationError,
            lambda: make_service_dict('foo', options, 'tests/fixtures/env'),
        )

    @mock.patch.dict(os.environ)
    def test_resolve_environment_from_file(self):
        os.environ['FILE_DEF'] = 'E1'
        os.environ['FILE_DEF_EMPTY'] = 'E2'
        os.environ['ENV_DEF'] = 'E3'
        service_dict = make_service_dict(
            'foo',
            {'env_file': 'resolve.env'},
            'tests/fixtures/env',
        )
        self.assertEqual(
            service_dict['environment'],
            {'FILE_DEF': 'F1', 'FILE_DEF_EMPTY': '', 'ENV_DEF': 'E3', 'NO_DEF': ''},
        )

    @mock.patch.dict(os.environ)
    def test_resolve_path(self):
        os.environ['HOSTENV'] = '/tmp'
        os.environ['CONTAINERENV'] = '/host/tmp'

        service_dict = config.load(
            config.ConfigDetails(
                config={'foo': {'build': '.', 'volumes': ['$HOSTENV:$CONTAINERENV']}},
                working_dir="tests/fixtures/env",
                filename=None,
            )
        )[0]
        self.assertEqual(set(service_dict['volumes']), set(['/tmp:/host/tmp']))

        service_dict = config.load(
            config.ConfigDetails(
                config={'foo': {'build': '.', 'volumes': ['/opt${HOSTENV}:/opt${CONTAINERENV}']}},
                working_dir="tests/fixtures/env",
                filename=None,
            )
        )[0]
        self.assertEqual(set(service_dict['volumes']), set(['/opt/tmp:/opt/host/tmp']))


def load_from_filename(filename):
    return config.load(config.find('.', filename))


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
        try:
            load_from_filename('tests/fixtures/extends/circle-1.yml')
            raise Exception("Expected config.CircularReference to be raised")
        except config.CircularReference as e:
            self.assertEqual(
                [(os.path.basename(filename), service_name) for (filename, service_name) in e.trail],
                [
                    ('circle-1.yml', 'web'),
                    ('circle-2.yml', 'web'),
                    ('circle-1.yml', 'web'),
                ],
            )

    def test_extends_validation_empty_dictionary(self):
        with self.assertRaisesRegexp(ConfigurationError, 'service'):
            config.load(
                config.ConfigDetails(
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
                config.ConfigDetails(
                    {
                        'web': {'image': 'busybox', 'extends': {'file': 'common.yml'}},
                    },
                    'tests/fixtures/extends',
                    'filename.yml'
                )
            )

    def test_extends_validation_invalid_key(self):
        expected_error_msg = "Unsupported config option for 'web' service: 'rogue_key'"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                config.ConfigDetails(
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
        expected_error_msg = "Service 'web' configuration key 'extends' 'file' contains an invalid type"
        with self.assertRaisesRegexp(ConfigurationError, expected_error_msg):
            config.load(
                config.ConfigDetails(
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
            config.ConfigDetails(
                {
                    'web': {'image': 'busybox', 'extends': {'service': 'web', 'file': 'common.yml'}},
                },
                'tests/fixtures/extends',
                'common.yml'
            )
        )

        self.assertEquals(len(service), 1)
        self.assertIsInstance(service[0], dict)

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

    def test_blacklisted_options(self):
        def load_config():
            return make_service_dict('myweb', {
                'extends': {
                    'file': 'whatever',
                    'service': 'web',
                }
            }, '.')

        with self.assertRaisesRegexp(ConfigurationError, 'links'):
            other_config = {'web': {'links': ['db']}}

            with mock.patch.object(config, 'load_yaml', return_value=other_config):
                print(load_config())

        with self.assertRaisesRegexp(ConfigurationError, 'volumes_from'):
            other_config = {'web': {'volumes_from': ['db']}}

            with mock.patch.object(config, 'load_yaml', return_value=other_config):
                print(load_config())

        with self.assertRaisesRegexp(ConfigurationError, 'net'):
            other_config = {'web': {'net': 'container:db'}}

            with mock.patch.object(config, 'load_yaml', return_value=other_config):
                print(load_config())

        other_config = {'web': {'net': 'host'}}

        with mock.patch.object(config, 'load_yaml', return_value=other_config):
            print(load_config())

    def test_volume_path(self):
        dicts = load_from_filename('tests/fixtures/volume-path/docker-compose.yml')

        paths = [
            '%s:/foo' % os.path.abspath('tests/fixtures/volume-path/common/foo'),
            '%s:/bar' % os.path.abspath('tests/fixtures/volume-path/bar'),
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


class BuildPathTest(unittest.TestCase):
    def setUp(self):
        self.abs_context_path = os.path.join(os.getcwd(), 'tests/fixtures/build-ctx')

    def test_nonexistent_path(self):
        with self.assertRaises(ConfigurationError):
            config.load(
                config.ConfigDetails(
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


class GetConfigPathTestCase(unittest.TestCase):

    files = [
        'docker-compose.yml',
        'docker-compose.yaml',
        'fig.yml',
        'fig.yaml',
    ]

    def test_get_config_path_default_file_in_basedir(self):
        files = self.files
        self.assertEqual('docker-compose.yml', get_config_filename_for_files(files[0:]))
        self.assertEqual('docker-compose.yaml', get_config_filename_for_files(files[1:]))
        self.assertEqual('fig.yml', get_config_filename_for_files(files[2:]))
        self.assertEqual('fig.yaml', get_config_filename_for_files(files[3:]))
        with self.assertRaises(config.ComposeFileNotFound):
            get_config_filename_for_files([])

    def test_get_config_path_default_file_in_parent_dir(self):
        """Test with files placed in the subdir"""
        files = self.files

        def get_config_in_subdir(files):
            return get_config_filename_for_files(files, subdir=True)

        self.assertEqual('docker-compose.yml', get_config_in_subdir(files[0:]))
        self.assertEqual('docker-compose.yaml', get_config_in_subdir(files[1:]))
        self.assertEqual('fig.yml', get_config_in_subdir(files[2:]))
        self.assertEqual('fig.yaml', get_config_in_subdir(files[3:]))
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
        return os.path.basename(config.get_config_path(base_dir))
    finally:
        shutil.rmtree(project_dir)
