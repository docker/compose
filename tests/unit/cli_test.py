from __future__ import unicode_literals
from __future__ import absolute_import
import logging
import os
import tempfile
import shutil
from .. import unittest

import docker
import mock

from compose.cli import main
from compose.cli.main import TopLevelCommand
from compose.cli.errors import ComposeFileNotFound
from compose.service import Service


class CLITestCase(unittest.TestCase):
    def test_default_project_name(self):
        cwd = os.getcwd()

        try:
            os.chdir('tests/fixtures/simple-composefile')
            command = TopLevelCommand()
            project_name = command.get_project_name(command.get_config_path())
            self.assertEquals('simplecomposefile', project_name)
        finally:
            os.chdir(cwd)

    def test_project_name_with_explicit_base_dir(self):
        command = TopLevelCommand()
        command.base_dir = 'tests/fixtures/simple-composefile'
        project_name = command.get_project_name(command.get_config_path())
        self.assertEquals('simplecomposefile', project_name)

    def test_project_name_with_explicit_uppercase_base_dir(self):
        command = TopLevelCommand()
        command.base_dir = 'tests/fixtures/UpperCaseDir'
        project_name = command.get_project_name(command.get_config_path())
        self.assertEquals('uppercasedir', project_name)

    def test_project_name_with_explicit_project_name(self):
        command = TopLevelCommand()
        name = 'explicit-project-name'
        project_name = command.get_project_name(None, project_name=name)
        self.assertEquals('explicitprojectname', project_name)

    def test_project_name_from_environment_old_var(self):
        command = TopLevelCommand()
        name = 'namefromenv'
        with mock.patch.dict(os.environ):
            os.environ['FIG_PROJECT_NAME'] = name
            project_name = command.get_project_name(None)
        self.assertEquals(project_name, name)

    def test_project_name_from_environment_new_var(self):
        command = TopLevelCommand()
        name = 'namefromenv'
        with mock.patch.dict(os.environ):
            os.environ['COMPOSE_PROJECT_NAME'] = name
            project_name = command.get_project_name(None)
        self.assertEquals(project_name, name)

    def test_filename_check(self):
        files = [
            'docker-compose.yml',
            'docker-compose.yaml',
            'fig.yml',
            'fig.yaml',
        ]

        """Test with files placed in the basedir"""

        self.assertEqual('docker-compose.yml', get_config_filename_for_files(files[0:]))
        self.assertEqual('docker-compose.yaml', get_config_filename_for_files(files[1:]))
        self.assertEqual('fig.yml', get_config_filename_for_files(files[2:]))
        self.assertEqual('fig.yaml', get_config_filename_for_files(files[3:]))
        self.assertRaises(ComposeFileNotFound, lambda: get_config_filename_for_files([]))

        """Test with files placed in the subdir"""

        def get_config_filename_for_files_in_subdir(files):
            return get_config_filename_for_files(files, subdir=True)

        self.assertEqual('docker-compose.yml', get_config_filename_for_files_in_subdir(files[0:]))
        self.assertEqual('docker-compose.yaml', get_config_filename_for_files_in_subdir(files[1:]))
        self.assertEqual('fig.yml', get_config_filename_for_files_in_subdir(files[2:]))
        self.assertEqual('fig.yaml', get_config_filename_for_files_in_subdir(files[3:]))
        self.assertRaises(ComposeFileNotFound, lambda: get_config_filename_for_files_in_subdir([]))

    def test_get_project(self):
        command = TopLevelCommand()
        command.base_dir = 'tests/fixtures/longer-filename-composefile'
        project = command.get_project(command.get_config_path())
        self.assertEqual(project.name, 'longerfilenamecomposefile')
        self.assertTrue(project.client)
        self.assertTrue(project.services)

    def test_help(self):
        command = TopLevelCommand()
        with self.assertRaises(SystemExit):
            command.dispatch(['-h'], None)

    def test_setup_logging(self):
        main.setup_logging()
        self.assertEqual(logging.getLogger().level, logging.DEBUG)
        self.assertEqual(logging.getLogger('requests').propagate, False)

    @mock.patch('compose.cli.main.dockerpty', autospec=True)
    def test_run_with_environment_merged_with_options_list(self, mock_dockerpty):
        command = TopLevelCommand()
        mock_client = mock.create_autospec(docker.Client)
        mock_project = mock.Mock()
        mock_project.get_service.return_value = Service(
            'service',
            client=mock_client,
            environment=['FOO=ONE', 'BAR=TWO'],
            image='someimage')

        command.run(mock_project, {
            'SERVICE': 'service',
            'COMMAND': None,
            '-e': ['BAR=NEW', 'OTHER=THREE'],
            '--user': None,
            '--no-deps': None,
            '--allow-insecure-ssl': None,
            '-d': True,
            '-T': None,
            '--entrypoint': None,
            '--service-ports': None,
            '--rm': None,
        })

        _, _, call_kwargs = mock_client.create_container.mock_calls[0]
        self.assertEqual(
            call_kwargs['environment'],
            {'FOO': 'ONE', 'BAR': 'NEW', 'OTHER': 'THREE'})

    def test_run_service_with_restart_always(self):
        command = TopLevelCommand()
        mock_client = mock.create_autospec(docker.Client)
        mock_project = mock.Mock()
        mock_project.get_service.return_value = Service(
            'service',
            client=mock_client,
            restart='always',
            image='someimage')
        command.run(mock_project, {
            'SERVICE': 'service',
            'COMMAND': None,
            '-e': [],
            '--user': None,
            '--no-deps': None,
            '--allow-insecure-ssl': None,
            '-d': True,
            '-T': None,
            '--entrypoint': None,
            '--service-ports': None,
            '--rm': None,
        })
        _, _, call_kwargs = mock_client.create_container.mock_calls[0]
        self.assertEquals(call_kwargs['host_config']['RestartPolicy']['Name'], 'always')

        command = TopLevelCommand()
        mock_client = mock.create_autospec(docker.Client)
        mock_project = mock.Mock()
        mock_project.get_service.return_value = Service(
            'service',
            client=mock_client,
            restart='always',
            image='someimage')
        command.run(mock_project, {
            'SERVICE': 'service',
            'COMMAND': None,
            '-e': [],
            '--user': None,
            '--no-deps': None,
            '--allow-insecure-ssl': None,
            '-d': True,
            '-T': None,
            '--entrypoint': None,
            '--service-ports': None,
            '--rm': True,
        })
        _, _, call_kwargs = mock_client.create_container.mock_calls[0]
        self.assertFalse('RestartPolicy' in call_kwargs['host_config'])


def get_config_filename_for_files(filenames, subdir=None):
    project_dir = tempfile.mkdtemp()
    try:
        make_files(project_dir, filenames)
        command = TopLevelCommand()
        if subdir:
            command.base_dir = tempfile.mkdtemp(dir=project_dir)
        else:
            command.base_dir = project_dir
        return os.path.basename(command.get_config_path())
    finally:
        shutil.rmtree(project_dir)


def make_files(dirname, filenames):
    for fname in filenames:
        with open(os.path.join(dirname, fname), 'w') as f:
            f.write('')
