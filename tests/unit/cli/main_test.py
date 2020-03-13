from __future__ import absolute_import
from __future__ import unicode_literals

import io
import logging
import tempfile

import docker
import pytest

from compose import container
from compose.cli.errors import UserError
from compose.cli.formatter import ConsoleWarningFormatter
from compose.cli.main import build_one_off_container_options
from compose.cli.main import call_docker
from compose.cli.main import convergence_strategy_from_opts
from compose.cli.main import filter_attached_containers
from compose.cli.main import get_docker_start_call
from compose.cli.main import perform_command
from compose.cli.main import setup_console_handler
from compose.cli.main import warn_for_swarm_mode
from compose.config.config import Config
from compose.config.config import ConfigDetails
from compose.config.config import ConfigFile
from compose.config.environment import Environment
from compose.const import COMPOSEFILE_V3_4
from compose.service import ConvergenceStrategy
from tests import mock


def mock_container(service, number):
    return mock.create_autospec(
        container.Container,
        service=service,
        number=number,
        name_without_project='{0}_{1}'.format(service, number))


@pytest.fixture
def logging_handler():
    stream = mock.Mock()
    stream.isatty.return_value = True
    return logging.StreamHandler(stream=stream)


class TestCLIMainTestCase(object):

    def test_filter_attached_containers(self):
        containers = [
            mock_container('web', 1),
            mock_container('web', 2),
            mock_container('db', 1),
            mock_container('other', 1),
            mock_container('another', 1),
        ]
        service_names = ['web', 'db']
        actual = filter_attached_containers(containers, service_names)
        assert actual == containers[:3]

    def test_filter_attached_containers_with_dependencies(self):
        containers = [
            mock_container('web', 1),
            mock_container('web', 2),
            mock_container('db', 1),
            mock_container('other', 1),
            mock_container('another', 1),
        ]
        service_names = ['web', 'db']
        actual = filter_attached_containers(containers, service_names, attach_dependencies=True)
        assert actual == containers

    def test_filter_attached_containers_all(self):
        containers = [
            mock_container('web', 1),
            mock_container('db', 1),
            mock_container('other', 1),
        ]
        service_names = []
        actual = filter_attached_containers(containers, service_names)
        assert actual == containers

    def test_warning_in_swarm_mode(self):
        mock_client = mock.create_autospec(docker.APIClient)
        mock_client.info.return_value = {'Swarm': {'LocalNodeState': 'active'}}

        with mock.patch('compose.cli.main.log') as fake_log:
            warn_for_swarm_mode(mock_client)
            assert fake_log.warning.call_count == 1

    def test_build_one_off_container_options(self):
        command = 'build myservice'
        detach = False
        options = {
            '-e': ['MYVAR=MYVALUE'],
            '-T': True,
            '--label': ['MYLABEL'],
            '--entrypoint': 'bash',
            '--user': 'MYUSER',
            '--service-ports': [],
            '--publish': '',
            '--name': 'MYNAME',
            '--workdir': '.',
            '--volume': [],
            'stdin_open': False,
        }

        expected_container_options = {
            'command': command,
            'tty': False,
            'stdin_open': False,
            'detach': detach,
            'entrypoint': 'bash',
            'environment': {'MYVAR': 'MYVALUE'},
            'labels': {'MYLABEL': ''},
            'name': 'MYNAME',
            'ports': [],
            'restart': None,
            'user': 'MYUSER',
            'working_dir': '.',
        }

        container_options = build_one_off_container_options(options, detach, command)
        assert container_options == expected_container_options

    @pytest.mark.parametrize('cli_build', [False, True])
    def test_build_native_args_propagated(self, cli_build):
        options = {
            '--build-arg': ['MYVAR', 'ARG=123'],
            '--no-cache': True,
            '--pull': True,
            '--force-rm': True,
            '--memory': True,
            '--compress': True,
            '--parallel': True,
            '--quiet': True,
            '--progress': 'progress',
            'SERVICE': ['service'],
            'COMMAND': 'build',
        }
        env = Environment({
            'MYVAR': 'MYVALUE',
        })
        if cli_build:
            env['COMPOSE_DOCKER_CLI_BUILD'] = '1'
        with mock.patch('compose.cli.main.TopLevelCommand.toplevel_environment', new=env), \
                mock.patch('compose.cli.main.Environment.from_env_file', return_value=env), \
                mock.patch('compose.project.Project.build') as mock_build, \
                mock.patch('compose.cli.command.config.find') as mock_config_find, \
                mock.patch('compose.cli.command.config.load') as mock_config_load:
            mock_config_find.return_value = ConfigDetails(
                working_dir='working_dir',
                config_files=[ConfigFile(filename='config_file', config={})],
                environment=env,
            )
            mock_config_load.return_value = Config(
                version=COMPOSEFILE_V3_4,
                services=[],
                volumes={},
                networks={},
                secrets={},
                configs={},
            )
            project = [None]

            def handler(command, options):
                project[0] = command.project
                command.build(options)

            perform_command(options, handler=handler, command_options=options)
            assert mock_build.call_args == mock.call(
                service_names=['service'],
                no_cache=True,
                pull=True,
                force_rm=True,
                memory=True,
                rm=True,
                build_args={'MYVAR': 'MYVALUE', 'ARG': '123'},
                gzip=True,
                parallel_build=True,
                silent=True,
                progress='progress',
            )
            assert project[0].native_build_enabled == bool(cli_build)

    @pytest.mark.parametrize('cli_build', [False, True])
    def test_build_native_builder_called(self, cli_build):
        options = {
            '--build-arg': ['MYVAR', 'ARG=123'],
            '--no-cache': True,
            '--pull': True,
            '--force-rm': False,
            '--memory': True,
            '--compress': False,
            '--parallel': False,
            '--quiet': True,
            '--progress': 'progress',
            'SERVICE': ['service'],
            'COMMAND': 'build',
        }
        env = Environment({
            'MYVAR': 'MYVALUE',
        })

        if cli_build:
            env['COMPOSE_DOCKER_CLI_BUILD'] = '1'
            env['COMPOSE_DOCKER_CLI_BUILD_EXTRA_ARGS'] = '--extra0 --extra1=1'

        iidfile = [None]

        def mock_mktemp():
            iidfile[0] = tempfile.mktemp()
            with open(iidfile[0], 'w') as f:
                f.write(':12345')
            return iidfile[0]

        with mock.patch('compose.cli.main.TopLevelCommand.toplevel_environment', new=env), \
                mock.patch('compose.cli.main.Environment.from_env_file', return_value=env), \
                mock.patch('compose.service.subprocess.Popen') as mock_subprocess_popen, \
                mock.patch('compose.service.tempfile', new=mock.Mock(mktemp=mock_mktemp)), \
                mock.patch('compose.cli.command.get_client') as mock_get_client, \
                mock.patch('compose.cli.command.config.find') as mock_config_find, \
                mock.patch('compose.cli.command.config.load') as mock_config_load:
            mock_config_find.return_value = ConfigDetails(
                working_dir='working_dir',
                config_files=[ConfigFile(filename='config_file', config={})],
                environment=env,
            )
            mock_config_load.return_value = Config(
                version=COMPOSEFILE_V3_4,
                services=[{
                    'name': 'service',
                    'build': {
                        'context': '.',
                    },
                }],
                volumes={},
                networks={},
                secrets={},
                configs={},
            )
            mock_get_client.return_value.api_version = '1.35'
            mock_build = mock_get_client.return_value.build
            mock_build.return_value = \
                mock_subprocess_popen.return_value.__enter__.return_value.stdout = \
                io.StringIO('{"stream": "Successfully built 12345"}')

            project = [None]

            def handler(command, options):
                project[0] = command.project
                command.build(options)

            perform_command(options, handler=handler, command_options=options)
            if not cli_build:
                assert mock_build.called
                assert mock_build.call_args[1]['buildargs'] == {'MYVAR': 'MYVALUE', 'ARG': '123'}
                assert mock_build.call_args[1]['pull']
                assert mock_build.call_args[1]['nocache']
                assert not mock_build.call_args[1]['forcerm']
                assert not mock_build.call_args[1]['gzip']
                assert not project[0].native_build_enabled
            else:
                assert mock_subprocess_popen.call_args[0][0] == [
                    'docker',
                    'build',
                    '--build-arg', 'MYVAR=MYVALUE',
                    '--build-arg', 'ARG=123',
                    '--memory', 'True',
                    '--no-cache',
                    '--progress', 'progress',
                    '--pull',
                    '--tag', 'working_dir_service',
                    '--iidfile', iidfile[0],
                    '--extra0',
                    '--extra1=1',
                    '.',
                ]
                assert project[0].native_build_enabled

    def test_get_docker_start_call(self):
        container_id = 'my_container_id'

        mock_container_options = {'detach': False, 'stdin_open': True}
        expected_docker_start_call = ['start', '--attach', '--interactive', container_id]
        docker_start_call = get_docker_start_call(mock_container_options, container_id)
        assert expected_docker_start_call == docker_start_call

        mock_container_options = {'detach': False, 'stdin_open': False}
        expected_docker_start_call = ['start', '--attach', container_id]
        docker_start_call = get_docker_start_call(mock_container_options, container_id)
        assert expected_docker_start_call == docker_start_call

        mock_container_options = {'detach': True, 'stdin_open': True}
        expected_docker_start_call = ['start', '--interactive', container_id]
        docker_start_call = get_docker_start_call(mock_container_options, container_id)
        assert expected_docker_start_call == docker_start_call

        mock_container_options = {'detach': True, 'stdin_open': False}
        expected_docker_start_call = ['start', container_id]
        docker_start_call = get_docker_start_call(mock_container_options, container_id)
        assert expected_docker_start_call == docker_start_call


class TestSetupConsoleHandlerTestCase(object):

    def test_with_tty_verbose(self, logging_handler):
        setup_console_handler(logging_handler, True)
        assert type(logging_handler.formatter) == ConsoleWarningFormatter
        assert '%(name)s' in logging_handler.formatter._fmt
        assert '%(funcName)s' in logging_handler.formatter._fmt

    def test_with_tty_not_verbose(self, logging_handler):
        setup_console_handler(logging_handler, False)
        assert type(logging_handler.formatter) == ConsoleWarningFormatter
        assert '%(name)s' not in logging_handler.formatter._fmt
        assert '%(funcName)s' not in logging_handler.formatter._fmt

    def test_with_not_a_tty(self, logging_handler):
        logging_handler.stream.isatty.return_value = False
        setup_console_handler(logging_handler, False)
        assert type(logging_handler.formatter) == logging.Formatter


class TestConvergeStrategyFromOptsTestCase(object):

    def test_invalid_opts(self):
        options = {'--force-recreate': True, '--no-recreate': True}
        with pytest.raises(UserError):
            convergence_strategy_from_opts(options)

    def test_always(self):
        options = {'--force-recreate': True, '--no-recreate': False}
        assert (
            convergence_strategy_from_opts(options) ==
            ConvergenceStrategy.always
        )

    def test_never(self):
        options = {'--force-recreate': False, '--no-recreate': True}
        assert (
            convergence_strategy_from_opts(options) ==
            ConvergenceStrategy.never
        )

    def test_changed(self):
        options = {'--force-recreate': False, '--no-recreate': False}
        assert (
            convergence_strategy_from_opts(options) ==
            ConvergenceStrategy.changed
        )


def mock_find_executable(exe):
    return exe


@mock.patch('compose.cli.main.find_executable', mock_find_executable)
class TestCallDocker(object):
    def test_simple_no_options(self):
        with mock.patch('subprocess.call') as fake_call:
            call_docker(['ps'], {}, {})

        assert fake_call.call_args[0][0] == ['docker', 'ps']

    def test_simple_tls_option(self):
        with mock.patch('subprocess.call') as fake_call:
            call_docker(['ps'], {'--tls': True}, {})

        assert fake_call.call_args[0][0] == ['docker', '--tls', 'ps']

    def test_advanced_tls_options(self):
        with mock.patch('subprocess.call') as fake_call:
            call_docker(['ps'], {
                '--tls': True,
                '--tlscacert': './ca.pem',
                '--tlscert': './cert.pem',
                '--tlskey': './key.pem',
            }, {})

        assert fake_call.call_args[0][0] == [
            'docker', '--tls', '--tlscacert', './ca.pem', '--tlscert',
            './cert.pem', '--tlskey', './key.pem', 'ps'
        ]

    def test_with_host_option(self):
        with mock.patch('subprocess.call') as fake_call:
            call_docker(['ps'], {'--host': 'tcp://mydocker.net:2333'}, {})

        assert fake_call.call_args[0][0] == [
            'docker', '--host', 'tcp://mydocker.net:2333', 'ps'
        ]

    def test_with_http_host(self):
        with mock.patch('subprocess.call') as fake_call:
            call_docker(['ps'], {'--host': 'http://mydocker.net:2333'}, {})

        assert fake_call.call_args[0][0] == [
            'docker', '--host', 'tcp://mydocker.net:2333', 'ps',
        ]

    def test_with_host_option_shorthand_equal(self):
        with mock.patch('subprocess.call') as fake_call:
            call_docker(['ps'], {'--host': '=tcp://mydocker.net:2333'}, {})

        assert fake_call.call_args[0][0] == [
            'docker', '--host', 'tcp://mydocker.net:2333', 'ps'
        ]

    def test_with_env(self):
        with mock.patch('subprocess.call') as fake_call:
            call_docker(['ps'], {}, {'DOCKER_HOST': 'tcp://mydocker.net:2333'})

        assert fake_call.call_args[0][0] == [
            'docker', 'ps'
        ]
        assert fake_call.call_args[1]['env'] == {'DOCKER_HOST': 'tcp://mydocker.net:2333'}
