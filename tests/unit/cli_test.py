# encoding: utf-8
from __future__ import absolute_import
from __future__ import unicode_literals

import os
import shutil
import tempfile
from io import StringIO

import docker
import py
import pytest
from docker.constants import DEFAULT_DOCKER_API_VERSION

from .. import mock
from .. import unittest
from ..helpers import build_config
from compose.cli.command import get_project
from compose.cli.command import get_project_name
from compose.cli.docopt_command import NoSuchCommand
from compose.cli.errors import UserError
from compose.cli.main import TopLevelCommand
from compose.const import IS_WINDOWS_PLATFORM
from compose.project import Project


class CLITestCase(unittest.TestCase):

    def test_default_project_name(self):
        test_dir = py._path.local.LocalPath('tests/fixtures/simple-composefile')
        with test_dir.as_cwd():
            project_name = get_project_name('.')
            assert 'simple-composefile' == project_name

    def test_project_name_with_explicit_base_dir(self):
        base_dir = 'tests/fixtures/simple-composefile'
        project_name = get_project_name(base_dir)
        assert 'simple-composefile' == project_name

    def test_project_name_with_explicit_uppercase_base_dir(self):
        base_dir = 'tests/fixtures/UpperCaseDir'
        project_name = get_project_name(base_dir)
        assert 'uppercasedir' == project_name

    def test_project_name_with_explicit_project_name(self):
        name = 'explicit-project-name'
        project_name = get_project_name(None, project_name=name)
        assert 'explicit-project-name' == project_name

    @mock.patch.dict(os.environ)
    def test_project_name_from_environment_new_var(self):
        name = 'namefromenv'
        os.environ['COMPOSE_PROJECT_NAME'] = name
        project_name = get_project_name(None)
        assert project_name == name

    def test_project_name_with_empty_environment_var(self):
        base_dir = 'tests/fixtures/simple-composefile'
        with mock.patch.dict(os.environ):
            os.environ['COMPOSE_PROJECT_NAME'] = ''
            project_name = get_project_name(base_dir)
        assert 'simple-composefile' == project_name

    @mock.patch.dict(os.environ)
    def test_project_name_with_environment_file(self):
        base_dir = tempfile.mkdtemp()
        try:
            name = 'namefromenvfile'
            with open(os.path.join(base_dir, '.env'), 'w') as f:
                f.write('COMPOSE_PROJECT_NAME={}'.format(name))
            project_name = get_project_name(base_dir)
            assert project_name == name

            # Environment has priority over .env file
            os.environ['COMPOSE_PROJECT_NAME'] = 'namefromenv'
            assert get_project_name(base_dir) == os.environ['COMPOSE_PROJECT_NAME']
        finally:
            shutil.rmtree(base_dir)

    def test_get_project(self):
        base_dir = 'tests/fixtures/longer-filename-composefile'
        project = get_project(base_dir)
        assert project.name == 'longer-filename-composefile'
        assert project.client
        assert project.services

    def test_command_help(self):
        with mock.patch('sys.stdout', new=StringIO()) as fake_stdout:
            TopLevelCommand.help({'COMMAND': 'up'})

        assert "Usage: up" in fake_stdout.getvalue()

    def test_command_help_nonexistent(self):
        with pytest.raises(NoSuchCommand):
            TopLevelCommand.help({'COMMAND': 'nonexistent'})

    @pytest.mark.xfail(IS_WINDOWS_PLATFORM, reason="requires dockerpty")
    @mock.patch('compose.cli.main.RunOperation', autospec=True)
    @mock.patch('compose.cli.main.PseudoTerminal', autospec=True)
    @mock.patch.dict(os.environ)
    def test_run_interactive_passes_logs_false(self, mock_pseudo_terminal, mock_run_operation):
        os.environ['COMPOSE_INTERACTIVE_NO_CLI'] = 'true'
        mock_client = mock.create_autospec(docker.APIClient)
        mock_client.api_version = DEFAULT_DOCKER_API_VERSION
        mock_client._general_configs = {}
        project = Project.from_config(
            name='composetest',
            client=mock_client,
            config_data=build_config({
                'service': {'image': 'busybox'}
            }),
        )
        command = TopLevelCommand(project)

        with pytest.raises(SystemExit):
            command.run({
                'SERVICE': 'service',
                'COMMAND': None,
                '-e': [],
                '--label': [],
                '--user': None,
                '--no-deps': None,
                '--detach': False,
                '-T': None,
                '--entrypoint': None,
                '--service-ports': None,
                '--use-aliases': None,
                '--publish': [],
                '--volume': [],
                '--rm': None,
                '--name': None,
                '--workdir': None,
            })

        _, _, call_kwargs = mock_run_operation.mock_calls[0]
        assert call_kwargs['logs'] is False

    def test_run_service_with_restart_always(self):
        mock_client = mock.create_autospec(docker.APIClient)
        mock_client.api_version = DEFAULT_DOCKER_API_VERSION
        mock_client._general_configs = {}

        project = Project.from_config(
            name='composetest',
            client=mock_client,
            config_data=build_config({
                'service': {
                    'image': 'busybox',
                    'restart': 'always',
                }
            }),
        )

        command = TopLevelCommand(project)
        command.run({
            'SERVICE': 'service',
            'COMMAND': None,
            '-e': [],
            '--label': [],
            '--user': None,
            '--no-deps': None,
            '--detach': True,
            '-T': None,
            '--entrypoint': None,
            '--service-ports': None,
            '--use-aliases': None,
            '--publish': [],
            '--volume': [],
            '--rm': None,
            '--name': None,
            '--workdir': None,
        })

        # NOTE: The "run" command is supposed to be a one-off tool; therefore restart policy "no"
        #       (the default) is enforced despite explicit wish for "always" in the project
        #       configuration file
        assert not mock_client.create_host_config.call_args[1].get('restart_policy')

        command = TopLevelCommand(project)
        command.run({
            'SERVICE': 'service',
            'COMMAND': None,
            '-e': [],
            '--label': [],
            '--user': None,
            '--no-deps': None,
            '--detach': True,
            '-T': None,
            '--entrypoint': None,
            '--service-ports': None,
            '--use-aliases': None,
            '--publish': [],
            '--volume': [],
            '--rm': True,
            '--name': None,
            '--workdir': None,
        })

        assert not mock_client.create_host_config.call_args[1].get('restart_policy')

    def test_command_manual_and_service_ports_together(self):
        project = Project.from_config(
            name='composetest',
            client=None,
            config_data=build_config({
                'service': {'image': 'busybox'},
            }),
        )
        command = TopLevelCommand(project)

        with pytest.raises(UserError):
            command.run({
                'SERVICE': 'service',
                'COMMAND': None,
                '-e': [],
                '--label': [],
                '--user': None,
                '--no-deps': None,
                '--detach': True,
                '-T': None,
                '--entrypoint': None,
                '--service-ports': True,
                '--use-aliases': None,
                '--publish': ['80:80'],
                '--rm': None,
                '--name': None,
            })
