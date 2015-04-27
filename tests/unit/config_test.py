import os
import mock
from tempfile import NamedTemporaryFile
from .. import unittest

from compose import config
import compose.errors
import compose.utils


class ConfigTest(unittest.TestCase):
    def test_from_dictionary(self):
        service_dicts = config.from_dictionary({
            'foo': {'image': 'busybox'},
            'bar': {'image': 'busybox', 'environment': ['FOO=1']},
        })

        self.assertListEqual(
            sorted(service_dicts, key=lambda d: d['name']),
            [{
                'environment': {'FOO': '1'},
                'name': 'bar',
                'image': 'busybox',
            },
                {
                'name': 'foo',
                'image': 'busybox',
            }]
        )

    def test_from_dictionary_throws_error_when_not_dict(self):
        with self.assertRaises(compose.errors.ConfigurationError):
            config.from_dictionary({
                'web': 'busybox:latest',
            })

    def test_config_validation(self):
        self.assertRaises(
            compose.errors.ValidationError,
            lambda: config.make_service_dict('foo', {'image': 'busybox', 'port': ['8000']})
        )
        config.make_service_dict('foo', {'image': 'busybox', 'ports': ['8000']})


class VolumePathTest(unittest.TestCase):
    @mock.patch.dict(os.environ)
    def test_volume_binding_with_environ(self):
        with NamedTemporaryFile('r') as path:
            os.environ['VOLUME_PATH'] = path.name
            d = config.make_service_dict('foo', {'image': 'busybox', 'volumes': ['${VOLUME_PATH}:/container/path']}, working_dir='.')
            self.assertEqual(d['volumes'], [path.name + ':/container/path'])

    @mock.patch.dict(os.environ)
    def test_volume_binding_with_home(self):
        d = config.make_service_dict('foo', {'image': 'busybox', 'volumes': ['~:/container/path']}, working_dir='.')
        self.assertEqual(d['volumes'], [os.environ['HOME'] + ':/container/path'])


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
            config.make_service_dict('foo', {'image': 'busybox', 'labels': ['foo=1', 'bar']}),
            config.make_service_dict('foo', {'image': 'busybox'}),
        )
        self.assertEqual(service_dict['labels'], {'foo': '1', 'bar': ''})

    def test_no_base(self):
        service_dict = config.merge_service_dicts(
            config.make_service_dict('foo', {'image': 'busybox'}),
            config.make_service_dict('foo', {'image': 'busybox', 'labels': ['foo=2']}),
        )
        self.assertEqual(service_dict['labels'], {'foo': '2'})

    def test_override_explicit_value(self):
        service_dict = config.merge_service_dicts(
            config.make_service_dict('foo', {'image': 'busybox', 'labels': ['foo=1', 'bar']}),
            config.make_service_dict('foo', {'image': 'busybox', 'labels': ['foo=2']}),
        )
        self.assertEqual(service_dict['labels'], {'foo': '2', 'bar': ''})

    def test_add_explicit_value(self):
        service_dict = config.merge_service_dicts(
            config.make_service_dict('foo', {'image': 'busybox', 'labels': ['foo=1', 'bar']}),
            config.make_service_dict('foo', {'image': 'busybox', 'labels': ['bar=2']}),
        )
        self.assertEqual(service_dict['labels'], {'foo': '1', 'bar': '2'})

    def test_remove_explicit_value(self):
        service_dict = config.merge_service_dicts(
            config.make_service_dict('foo', {'image': 'busybox', 'labels': ['foo=1', 'bar=2']}),
            config.make_service_dict('foo', {'image': 'busybox', 'labels': ['bar']}),
        )
        self.assertEqual(service_dict['labels'], {'foo': '1', 'bar': ''})


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
        with self.assertRaises(compose.errors.ConfigurationError):
            config.parse_environment('a=b')

    def test_parse_environment_empty(self):
        self.assertEqual(config.parse_environment(None), {})

    @mock.patch.dict(os.environ)
    def test_resolve_environment(self):
        os.environ['FILE_DEF'] = 'E1'
        os.environ['FILE_DEF_EMPTY'] = 'E2'
        os.environ['ENV_DEF'] = 'E3'

        service_dict = config.make_service_dict(
            'foo', {
                'image': 'busybox',
                'environment': {
                    'FILE_DEF': 'F1',
                    'FILE_DEF_EMPTY': '',
                    'ENV_DEF': None,
                    'NO_DEF': None
                },
            },
        )

        self.assertEqual(
            service_dict['environment'],
            {'FILE_DEF': 'F1', 'FILE_DEF_EMPTY': '', 'ENV_DEF': 'E3', 'NO_DEF': ''},
        )

    def test_env_from_file(self):
        service_dict = config.make_service_dict(
            'foo',
            {'image': 'busybox', 'env_file': 'one.env'},
            'tests/fixtures/env',
        )
        self.assertEqual(
            service_dict['environment'],
            {'ONE': '2', 'TWO': '1', 'THREE': '3', 'FOO': 'bar'},
        )

    def test_env_from_multiple_files(self):
        service_dict = config.make_service_dict(
            'foo',
            {'image': 'busybox', 'env_file': ['one.env', 'two.env']},
            'tests/fixtures/env',
        )
        self.assertEqual(
            service_dict['environment'],
            {'ONE': '2', 'TWO': '1', 'THREE': '3', 'FOO': 'baz', 'DOO': 'dah'},
        )

    def test_env_nonexistent_file(self):
        options = {'image': 'busybox', 'env_file': 'nonexistent.env'}
        self.assertRaises(
            compose.errors.ConfigurationError,
            lambda: config.make_service_dict('foo', options, 'tests/fixtures/env'),
        )

    @mock.patch.dict(os.environ)
    def test_resolve_environment_from_file(self):
        os.environ['FILE_DEF'] = 'E1'
        os.environ['FILE_DEF_EMPTY'] = 'E2'
        os.environ['ENV_DEF'] = 'E3'
        service_dict = config.make_service_dict(
            'foo',
            {'image': 'busybox', 'env_file': 'resolve.env'},
            'tests/fixtures/env',
        )
        self.assertEqual(
            service_dict['environment'],
            {'FILE_DEF': 'F1', 'FILE_DEF_EMPTY': '', 'ENV_DEF': 'E3', 'NO_DEF': ''},
        )


class ExtendsTest(unittest.TestCase):
    def test_extends(self):
        service_dicts = config.load('tests/fixtures/extends/docker-compose.yml')

        service_dicts = sorted(
            service_dicts,
            key=lambda sd: sd['name'],
        )

        self.assertEqual(service_dicts, [
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
        ])

    def test_nested(self):
        service_dicts = config.load('tests/fixtures/extends/nested.yml')

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

    def test_circular(self):
        try:
            config.load('tests/fixtures/extends/circle-1.yml')
            raise Exception("Expected config.CircularReference to be raised")
        except compose.errors.CircularReference as e:
            self.assertEqual(
                [(os.path.basename(filename), service_name) for (filename, service_name) in e.trail],
                [
                    ('circle-1.yml', 'web'),
                    ('circle-2.yml', 'web'),
                    ('circle-1.yml', 'web'),
                ],
            )

    def test_extends_validation(self):
        dictionary = {'extends': None}

        def load_config():
            return config.make_service_dict('myweb', dictionary, working_dir='tests/fixtures/extends')

        self.assertRaisesRegexp(compose.errors.ValidationError, 'dict', load_config)

        dictionary['extends'] = {}
        self.assertRaises(compose.errors.ValidationError, load_config)

        dictionary['extends']['file'] = 'common.yml'
        self.assertRaisesRegexp(compose.errors.ValidationError, 'service', load_config)

        dictionary['extends']['service'] = 'web'
        self.assertIsInstance(load_config(), dict)

        dictionary['extends']['what'] = 'is this'
        self.assertRaisesRegexp(compose.errors.ValidationError, 'must be given', load_config)

    def test_blacklisted_options(self):
        def load_config():
            return config.make_service_dict('myweb', {
                'image': 'busybox',
                'extends': {
                    'file': 'whatever',
                    'service': 'web',
                }
            }, '.')

        with self.assertRaisesRegexp(compose.errors.ValidationError, 'links'):
            other_config = {'web': {'links': ['db']}}

            with mock.patch.object(config, 'load_yaml', return_value=other_config):
                print load_config()

        with self.assertRaisesRegexp(compose.errors.ValidationError, 'volumes_from'):
            other_config = {'web': {'volumes_from': ['db']}}

            with mock.patch.object(config, 'load_yaml', return_value=other_config):
                print load_config()

        with self.assertRaisesRegexp(compose.errors.ValidationError, 'net'):
            other_config = {'web': {'net': 'container:db'}}

            with mock.patch.object(config, 'load_yaml', return_value=other_config):
                print load_config()

        other_config = {'web': {'net': 'host'}}

        with mock.patch.object(config, 'load_yaml', return_value=other_config):
            print load_config()

    def test_volume_path(self):
        dicts = config.load('tests/fixtures/volume-path/docker-compose.yml')

        paths = set([
            '%s:/foo' % os.path.abspath('tests/fixtures/volume-path/common/foo'),
            '%s:/bar' % os.path.abspath('tests/fixtures/volume-path/bar'),
        ])

        self.assertSetEqual(set(dicts[0]['volumes']), paths)

    def test_parent_build_path_dne(self):
        child = config.load('tests/fixtures/extends/nonexistent-path-child.yml')

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


class BuildPathTest(unittest.TestCase):
    def setUp(self):
        self.abs_context_path = os.path.join(os.getcwd(), 'tests/fixtures/build-ctx')

    def test_relative_path(self):
        relative_build_path = '../build-ctx/'
        service_dict = config.make_service_dict(
            'relpath',
            {'build': relative_build_path},
            working_dir='tests/fixtures/build-path'
        )
        self.assertEquals(service_dict['build'], self.abs_context_path)

    def test_absolute_path(self):
        service_dict = config.make_service_dict(
            'abspath',
            {'build': self.abs_context_path},
            working_dir='tests/fixtures/build-path'
        )
        self.assertEquals(service_dict['build'], self.abs_context_path)

    def test_from_file(self):
        service_dict = config.load('tests/fixtures/build-path/docker-compose.yml')
        self.assertEquals(service_dict, [{'name': 'foo', 'build': self.abs_context_path}])
