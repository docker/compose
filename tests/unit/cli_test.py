from __future__ import unicode_literals
from __future__ import absolute_import
from .. import unittest
from fig.cli.main import TopLevelCommand
from fig.packages.six import StringIO

class CLITestCase(unittest.TestCase):
    def test_project_name_defaults_to_dirname(self):
        command = TopLevelCommand()
        command.base_dir = 'tests/fixtures/simple-figfile'
        self.assertEquals('simplefigfile', command.project_name)

    def test_yaml_filename_check(self):
        command = TopLevelCommand()
        command.base_dir = 'tests/fixtures/longer-filename-figfile'
        self.assertTrue(command.project.get_service('definedinyamlnotyml'))

    def test_help(self):
        command = TopLevelCommand()
        with self.assertRaises(SystemExit):
            command.dispatch(['-h'], None)
