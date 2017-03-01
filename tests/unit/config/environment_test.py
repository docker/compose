# encoding: utf-8
from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

from compose.config.environment import Environment
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
