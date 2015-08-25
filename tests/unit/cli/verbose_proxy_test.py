from __future__ import absolute_import
from __future__ import unicode_literals

import six

from compose.cli import verbose_proxy
from tests import unittest


class VerboseProxyTestCase(unittest.TestCase):

    def test_format_call(self):
        prefix = '' if six.PY3 else 'u'
        expected = "(%(p)s'arg1', True, key=%(p)s'value')" % dict(p=prefix)
        actual = verbose_proxy.format_call(
            ("arg1", True),
            {'key': 'value'})

        self.assertEqual(expected, actual)

    def test_format_return_sequence(self):
        expected = "(list with 10 items)"
        actual = verbose_proxy.format_return(list(range(10)), 2)
        self.assertEqual(expected, actual)

    def test_format_return(self):
        expected = repr({'Id': 'ok'})
        actual = verbose_proxy.format_return({'Id': 'ok'}, 2)
        self.assertEqual(expected, actual)

    def test_format_return_no_result(self):
        actual = verbose_proxy.format_return(None, 2)
        self.assertEqual(None, actual)
