from __future__ import unicode_literals
from __future__ import absolute_import
from . import unittest
from fig.cli.main import TopLevelCommand

class CLITestCase(unittest.TestCase):
    def setUp(self):
        self.command = TopLevelCommand()
        self.command.base_dir = 'tests/fixtures/simple-figfile'

    def test_help(self):
        self.assertRaises(SystemExit, lambda: self.command.dispatch(['-h'], None))

    def test_ps(self):
        self.command.dispatch(['ps'], None)
