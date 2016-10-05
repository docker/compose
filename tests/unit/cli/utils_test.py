from __future__ import absolute_import
from __future__ import unicode_literals

import unittest

from compose.cli.utils import unquote_path


class UnquotePathTest(unittest.TestCase):
    def test_no_quotes(self):
        assert unquote_path('hello') == 'hello'

    def test_simple_quotes(self):
        assert unquote_path('"hello"') == 'hello'

    def test_uneven_quotes(self):
        assert unquote_path('"hello') == '"hello'
        assert unquote_path('hello"') == 'hello"'

    def test_nested_quotes(self):
        assert unquote_path('""hello""') == '"hello"'
        assert unquote_path('"hel"lo"') == 'hel"lo'
        assert unquote_path('"hello""') == 'hello"'
