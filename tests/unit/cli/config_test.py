from __future__ import unicode_literals
from __future__ import absolute_import
from tests import unittest

from compose.cli import config
from compose.cli.errors import UserError

import yaml
import os


TEST_YML_DICT_WITH_DEFAULTS = yaml.load("""db:
  image: postgres
web:
  build: ${DJANGO_BUILD:.}
  command: python manage.py runserver 0.0.0.0:8000
  volumes:
    - .:/code
  environment:
    DJANGO_ENV: ${TEST_VALUE:production}
  ports:
    - ${DJANGO_PORT:"8000:8000"}
    - ${PORT_A:80}:${PORT_B:80}
  links:
    - db""")

TEST_YML_DICT_NO_DEFAULTS = yaml.load("""db:
  image: postgres
web:
  build: ${TEST_VALUE}
  command: python manage.py runserver 0.0.0.0:8000
  volumes:
    - .:/code
  ports:
    - "8000:8000"
  links:
    - db""")


class ConfigTestCase(unittest.TestCase):

    def setUp(self):
        self.environment_variables = {
            "TEST_VALUE": os.environ.get('TEST_VALUE'),
            "PORT_A": os.environ.get('PORT_A'),
            "PORT_B": os.environ.get('PORT_B')
        }

    def tearDown(self):
        for variable, former_value in self.environment_variables.items():
            if former_value is not None:
                os.environ[variable] = former_value
            elif variable in os.environ:
                del os.environ[variable]

    def test_with_resolve_environment_vars_nonmatch_values(self):
        # It should just return non-string values
        self.assertEqual(1, config.resolve_environment_vars(1))
        self.assertEqual([], config.resolve_environment_vars([]))
        self.assertEqual({}, config.resolve_environment_vars({}))
        self.assertEqual(1.234, config.resolve_environment_vars(1.234))
        self.assertEqual(None, config.resolve_environment_vars(None))

        # Any string that doesn't match our regex should just be returned
        self.assertEqual('localhost', config.resolve_environment_vars('localhost'))

        expected = "some-other-host"
        os.environ['TEST_VALUE'] = expected

        # Bare mentions of the environment variable shouldn't work
        value = 'TEST_VALUE:foo'
        self.assertEqual(value, config.resolve_environment_vars(value))

        value = 'TEST_VALUE'
        self.assertEqual(value, config.resolve_environment_vars(value))

        # Incomplete pattern shouldn't work as well
        for value in ['${TEST_VALUE', '$TEST_VALUE', '{TEST_VALUE}']:
            self.assertEqual(value, config.resolve_environment_vars(value))
            value += ':foo'
            self.assertEqual(value, config.resolve_environment_vars(value))

    def test_fully_interpolated_matches(self):
        expected = "some-other-host"
        os.environ['TEST_VALUE'] = expected

        # if we have a basic match
        self.assertEqual(expected, config.resolve_environment_vars("${TEST_VALUE}"))

        # if we have a match with a default value
        self.assertEqual(expected, config.resolve_environment_vars("${TEST_VALUE:localhost}"))

        # escaping should prevent interpolation
        escaped_no_default = "\${TEST_VALUE}"
        escaped_with_default = "\${TEST_VALUE:localhost}"
        self.assertEqual(escaped_no_default, escaped_no_default)
        self.assertEqual(escaped_with_default, escaped_with_default)

        # if we have no match but a default value
        del os.environ['TEST_VALUE']
        self.assertEqual('localhost', config.resolve_environment_vars("${TEST_VALUE:localhost}"))

    def test_fully_interpolated_errors(self):
        if 'TEST_VALUE' in os.environ:
            del os.environ['TEST_VALUE']
        self.assertRaises(UserError, config.resolve_environment_vars, "${TEST_VALUE}")

    def test_functional_defaults_as_dict(self):
        d = config.with_environment_vars(TEST_YML_DICT_WITH_DEFAULTS)

        # tests the basic structure and functionality of defaults
        self.assertEqual(d['web']['build'], '.')

        # test that environment variables with defaults are handled in lists
        self.assertEqual(d['web']['ports'][0], '"8000:8000"')

        # test that environment variables with defaults are handled more than once in the same line
        self.assertEqual(d['web']['ports'][1], '80:80')

        # test that environment variables with defaults are handled with variables more than once in the same line
        os.environ['PORT_A'] = '8080'
        os.environ['PORT_B'] = '9000'
        d = config.with_environment_vars(TEST_YML_DICT_WITH_DEFAULTS)
        self.assertEqual(d['web']['ports'][1], '8080:9000')

        # test that environment variables with defaults are handled in dictionaries
        self.assertEqual(d['web']['environment']['DJANGO_ENV'], 'production')

        # test that having an environment variable set properly pulls it
        os.environ['TEST_VALUE'] = 'development'
        d = config.with_environment_vars(TEST_YML_DICT_WITH_DEFAULTS)
        self.assertEqual(d['web']['environment']['DJANGO_ENV'], 'development')

    def test_functional_no_defaults(self):
        # test that not having defaults raises an error in a real YML situation
        self.assertRaises(UserError, config.with_environment_vars, TEST_YML_DICT_NO_DEFAULTS)

        # test that a bare environment variable is interpolated
        # note that we have to reload it
        os.environ['TEST_VALUE'] = '/home/ubuntu/django'
        self.assertEqual(config.with_environment_vars(TEST_YML_DICT_NO_DEFAULTS)['web']['build'], '/home/ubuntu/django')