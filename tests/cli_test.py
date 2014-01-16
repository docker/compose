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

    def test_scale(self):
        project = self.command.project

        self.command.scale({'SERVICE=NUM': ['simple=1']})
        self.assertEqual(len(project.get_service('simple').containers()), 1)

        self.command.scale({'SERVICE=NUM': ['simple=3', 'another=2']})
        self.assertEqual(len(project.get_service('simple').containers()), 3)
        self.assertEqual(len(project.get_service('another').containers()), 2)

        self.command.scale({'SERVICE=NUM': ['simple=1', 'another=1']})
        self.assertEqual(len(project.get_service('simple').containers()), 1)
        self.assertEqual(len(project.get_service('another').containers()), 1)

        self.command.scale({'SERVICE=NUM': ['simple=1', 'another=1']})
        self.assertEqual(len(project.get_service('simple').containers()), 1)
        self.assertEqual(len(project.get_service('another').containers()), 1)

        self.command.scale({'SERVICE=NUM': ['simple=0', 'another=0']})
        self.assertEqual(len(project.get_service('simple').containers()), 0)
        self.assertEqual(len(project.get_service('another').containers()), 0)

