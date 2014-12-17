from __future__ import unicode_literals
from __future__ import absolute_import
import logging
import os
from .. import unittest

import mock

from fig.cli import main
from fig.cli.main import TopLevelCommand
from six import StringIO


class CLITestCase(unittest.TestCase):
    def test_default_project_name(self):
        cwd = os.getcwd()

        try:
            os.chdir('tests/fixtures/simple-figfile')
            command = TopLevelCommand()
            project_name = command.get_project_name(command.get_config_path())
            self.assertEquals('simplefigfile', project_name)
        finally:
            os.chdir(cwd)

    def test_project_name_with_explicit_base_dir(self):
        command = TopLevelCommand()
        command.base_dir = 'tests/fixtures/simple-figfile'
        project_name = command.get_project_name(command.get_config_path())
        self.assertEquals('simplefigfile', project_name)

    def test_project_name_with_explicit_project_name(self):
        command = TopLevelCommand()
        name = 'explicit-project-name'
        project_name = command.get_project_name(None, project_name=name)
        self.assertEquals('explicitprojectname', project_name)

    def test_project_name_from_environment(self):
        command = TopLevelCommand()
        name = 'namefromenv'
        with mock.patch.dict(os.environ):
            os.environ['FIG_PROJECT_NAME'] = name
            project_name = command.get_project_name(None)
        self.assertEquals(project_name, name)

    def test_yaml_filename_check(self):
        command = TopLevelCommand()
        command.base_dir = 'tests/fixtures/longer-filename-figfile'
        with mock.patch('fig.cli.command.log', autospec=True) as mock_log:
            self.assertTrue(command.get_config_path())
        self.assertEqual(mock_log.warning.call_count, 2)

    def test_get_project(self):
        command = TopLevelCommand()
        command.base_dir = 'tests/fixtures/longer-filename-figfile'
        project = command.get_project(command.get_config_path())
        self.assertEqual(project.name, 'longerfilenamefigfile')
        self.assertTrue(project.client)
        self.assertTrue(project.services)

    def test_help(self):
        command = TopLevelCommand()
        with self.assertRaises(SystemExit):
            command.dispatch(['-h'], None)

    def test_setup_logging(self):
        main.setup_logging()
        self.assertEqual(logging.getLogger().level, logging.DEBUG)
        self.assertEqual(logging.getLogger('requests').level, logging.WARN)
        self.assertEqual(logging.getLogger('boto').level, logging.WARN)
