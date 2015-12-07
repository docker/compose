# encoding: utf-8
from __future__ import absolute_import
from __future__ import unicode_literals

import os

import docker
import py
import pytest

from .. import mock
from .. import unittest
from compose.cli.command import get_project
from compose.cli.command import get_project_name
from compose.cli.docopt_command import NoSuchCommand
from compose.cli.errors import UserError
from compose.cli.main import TopLevelCommand
from compose.const import IS_WINDOWS_PLATFORM
from compose.service import Service


class CLITestCase(unittest.TestCase):

    def test_default_project_name(self):
        test_dir = py._path.local.LocalPath('tests/fixtures/simple-composefile')
        with test_dir.as_cwd():
            project_name = get_project_name('.')
            self.assertEquals('simplecomposefile', project_name)

    def test_project_name_with_explicit_base_dir(self):
        base_dir = 'tests/fixtures/simple-composefile'
        project_name = get_project_name(base_dir)
        self.assertEquals('simplecomposefile', project_name)

    def test_project_name_with_explicit_uppercase_base_dir(self):
        base_dir = 'tests/fixtures/UpperCaseDir'
        project_name = get_project_name(base_dir)
        self.assertEquals('uppercasedir', project_name)

    def test_project_name_with_explicit_project_name(self):
        name = 'explicit-project-name'
        project_name = get_project_name(None, project_name=name)
        self.assertEquals('explicitprojectname', project_name)

    def test_project_name_from_environment_old_var(self):
        name = 'namefromenv'
        with mock.patch.dict(os.environ):
            os.environ['FIG_PROJECT_NAME'] = name
            project_name = get_project_name(None)
        self.assertEquals(project_name, name)

    def test_project_name_from_environment_new_var(self):
        name = 'namefromenv'
        with mock.patch.dict(os.environ):
            os.environ['COMPOSE_PROJECT_NAME'] = name
            project_name = get_project_name(None)
        self.assertEquals(project_name, name)

    def test_get_project(self):
        base_dir = 'tests/fixtures/longer-filename-composefile'
        project = get_project(base_dir)
        self.assertEqual(project.name, 'longerfilenamecomposefile')
        self.assertTrue(project.client)
        self.assertTrue(project.services)

    def test_help(self):
        command = TopLevelCommand()
        with self.assertRaises(SystemExit):
            command.dispatch(['-h'], None)

    def test_command_help(self):
        with self.assertRaises(SystemExit) as ctx:
            TopLevelCommand().dispatch(['help', 'up'], None)

        self.assertIn('Usage: up', str(ctx.exception))

    def test_command_help_nonexistent(self):
        with self.assertRaises(NoSuchCommand):
            TopLevelCommand().dispatch(['help', 'nonexistent'], None)

    @pytest.mark.xfail(IS_WINDOWS_PLATFORM, reason="requires dockerpty")
    @mock.patch('compose.cli.main.dockerpty', autospec=True)
    def test_run_with_environment_merged_with_options_list(self, mock_dockerpty):
        command = TopLevelCommand()
        mock_client = mock.create_autospec(docker.Client)
        mock_project = mock.Mock(client=mock_client)
        mock_project.get_service.return_value = Service(
            'service',
            client=mock_client,
            environment=['FOO=ONE', 'BAR=TWO'],
            image='someimage')

        command.run(mock_project, {
            'SERVICE': 'service',
            'COMMAND': None,
            '-e': ['BAR=NEW', 'OTHER=bär'.encode('utf-8')],
            '--user': None,
            '--no-deps': None,
            '-d': True,
            '-T': None,
            '--entrypoint': None,
            '--service-ports': None,
            '--publish': [],
            '--rm': None,
            '--name': None,
        })

        _, _, call_kwargs = mock_client.create_container.mock_calls[0]
        self.assertEqual(
            call_kwargs['environment'],
            {'FOO': 'ONE', 'BAR': 'NEW', 'OTHER': u'bär'})

    def test_run_service_with_restart_always(self):
        command = TopLevelCommand()
        mock_client = mock.create_autospec(docker.Client)
        mock_project = mock.Mock(client=mock_client)
        mock_project.get_service.return_value = Service(
            'service',
            client=mock_client,
            restart={'Name': 'always', 'MaximumRetryCount': 0},
            image='someimage')
        command.run(mock_project, {
            'SERVICE': 'service',
            'COMMAND': None,
            '-e': [],
            '--user': None,
            '--no-deps': None,
            '-d': True,
            '-T': None,
            '--entrypoint': None,
            '--service-ports': None,
            '--publish': [],
            '--rm': None,
            '--name': None,
        })

        self.assertEquals(
            mock_client.create_host_config.call_args[1]['restart_policy']['Name'],
            'always'
        )

        command = TopLevelCommand()
        mock_client = mock.create_autospec(docker.Client)
        mock_project = mock.Mock(client=mock_client)
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
            '-d': True,
            '-T': None,
            '--entrypoint': None,
            '--service-ports': None,
            '--publish': [],
            '--rm': True,
            '--name': None,
        })

        self.assertFalse(
            mock_client.create_host_config.call_args[1].get('restart_policy')
        )

    def test_command_manula_and_service_ports_together(self):
        command = TopLevelCommand()
        mock_client = mock.create_autospec(docker.Client)
        mock_project = mock.Mock(client=mock_client)
        mock_project.get_service.return_value = Service(
            'service',
            client=mock_client,
            restart='always',
            image='someimage',
        )

        with self.assertRaises(UserError):
            command.run(mock_project, {
                'SERVICE': 'service',
                'COMMAND': None,
                '-e': [],
                '--user': None,
                '--no-deps': None,
                '-d': True,
                '-T': None,
                '--entrypoint': None,
                '--service-ports': True,
                '--publish': ['80:80'],
                '--rm': None,
                '--name': None,
            })
