# encoding: utf-8
from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import codecs

import pytest

from compose.config.environment import env_vars_from_file
from compose.config.environment import Environment
from compose.config.errors import ConfigurationError
from tests import unittest


class EnvironmentTest(unittest.TestCase):
    def test_get_simple(self):
        env = Environment({
            'FOO': 'bar',
            'BAR': '1',
            'BAZ': ''
        })

        assert env.get('FOO') == 'bar'
        assert env.get('BAR') == '1'
        assert env.get('BAZ') == ''

    def test_get_undefined(self):
        env = Environment({
            'FOO': 'bar'
        })
        assert env.get('FOOBAR') is None

    def test_get_boolean(self):
        env = Environment({
            'FOO': '',
            'BAR': '0',
            'BAZ': 'FALSE',
            'FOOBAR': 'true',
        })

        assert env.get_boolean('FOO') is False
        assert env.get_boolean('BAR') is False
        assert env.get_boolean('BAZ') is False
        assert env.get_boolean('FOOBAR') is True
        assert env.get_boolean('UNDEFINED') is False

    def test_env_vars_from_file_bom(self):
        tmpdir = pytest.ensuretemp('env_file')
        self.addCleanup(tmpdir.remove)
        with codecs.open('{}/bom.env'.format(str(tmpdir)), 'w', encoding='utf-8') as f:
            f.write('\ufeffPARK_BOM=박봄\n')
        assert env_vars_from_file(str(tmpdir.join('bom.env'))) == {
            'PARK_BOM': '박봄'
        }

    def test_env_vars_from_file_whitespace(self):
        tmpdir = pytest.ensuretemp('env_file')
        self.addCleanup(tmpdir.remove)
        with codecs.open('{}/whitespace.env'.format(str(tmpdir)), 'w', encoding='utf-8') as f:
            f.write('WHITESPACE =yes\n')
        with pytest.raises(ConfigurationError) as exc:
            env_vars_from_file(str(tmpdir.join('whitespace.env')))
        assert 'environment variable' in exc.exconly()
