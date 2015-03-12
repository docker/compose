import os
import mock
from .. import unittest

from compose import config

class ConfigTest(unittest.TestCase):
    def test_from_dictionary(self):
        service_dicts = config.from_dictionary({
            'foo': {'image': 'busybox'},
            'bar': {'environment': ['FOO=1']},
        })

        self.assertEqual(
            sorted(service_dicts, key=lambda d: d['name']),
            sorted([
                {
                    'name': 'bar',
                    'environment': {'FOO': '1'},
                },
                {
                    'name': 'foo',
                    'image': 'busybox',
                }
            ])
        )

    def test_from_dictionary_throws_error_when_not_dict(self):
        with self.assertRaises(config.ConfigurationError):
            config.from_dictionary({
                'web': 'busybox:latest',
            })

    def test_config_validation(self):
        self.assertRaises(
            config.ConfigurationError,
            lambda: config.make_service_dict('foo', {'port': ['8000']})
        )
        config.make_service_dict('foo', {'ports': ['8000']})

    def test_parse_environment_as_list(self):
        environment =[
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
        with self.assertRaises(config.ConfigurationError):
            config.parse_environment('a=b')

    def test_parse_environment_empty(self):
        self.assertEqual(config.parse_environment(None), {})

    @mock.patch.dict(os.environ)
    def test_resolve_environment(self):
        os.environ['FILE_DEF'] = 'E1'
        os.environ['FILE_DEF_EMPTY'] = 'E2'
        os.environ['ENV_DEF'] = 'E3'

        service_dict = config.make_service_dict(
            'foo',
            {
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
            {'env_file': 'one.env'},
            'tests/fixtures/env',
        )
        self.assertEqual(
            service_dict['environment'],
            {'ONE': '2', 'TWO': '1', 'THREE': '3', 'FOO': 'bar'},
        )

    def test_env_from_multiple_files(self):
        service_dict = config.make_service_dict(
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
            config.ConfigurationError,
            lambda: config.make_service_dict('foo', options, 'tests/fixtures/env'),
        )

    @mock.patch.dict(os.environ)
    def test_resolve_environment_from_file(self):
        os.environ['FILE_DEF'] = 'E1'
        os.environ['FILE_DEF_EMPTY'] = 'E2'
        os.environ['ENV_DEF'] = 'E3'
        service_dict = config.make_service_dict(
            'foo',
            {'env_file': 'resolve.env'},
            'tests/fixtures/env',
        )
        self.assertEqual(
            service_dict['environment'],
            {'FILE_DEF': 'F1', 'FILE_DEF_EMPTY': '', 'ENV_DEF': 'E3', 'NO_DEF': ''},
        )
