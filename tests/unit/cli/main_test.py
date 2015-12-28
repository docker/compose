from __future__ import absolute_import

import logging

from compose import container
from compose.cli.errors import UserError
from compose.cli.formatter import ConsoleWarningFormatter
from compose.cli.log_printer import LogPrinter
from compose.cli.main import attach_to_logs
from compose.cli.main import build_log_printer
from compose.cli.main import convergence_strategy_from_opts
from compose.cli.main import setup_console_handler
from compose.project import Project
from compose.service import ConvergenceStrategy
from tests import mock
from tests import unittest


def mock_container(service, number):
    return mock.create_autospec(
        container.Container,
        service=service,
        number=number,
        name_without_project='{0}_{1}'.format(service, number))


class CLIMainTestCase(unittest.TestCase):

    def test_build_log_printer(self):
        containers = [
            mock_container('web', 1),
            mock_container('web', 2),
            mock_container('db', 1),
            mock_container('other', 1),
            mock_container('another', 1),
        ]
        service_names = ['web', 'db']
        log_printer = build_log_printer(containers, service_names, True)
        self.assertEqual(log_printer.containers, containers[:3])

    def test_build_log_printer_all_services(self):
        containers = [
            mock_container('web', 1),
            mock_container('db', 1),
            mock_container('other', 1),
        ]
        service_names = []
        log_printer = build_log_printer(containers, service_names, True)
        self.assertEqual(log_printer.containers, containers)

    def test_attach_to_logs(self):
        project = mock.create_autospec(Project)
        log_printer = mock.create_autospec(LogPrinter, containers=[])
        service_names = ['web', 'db']
        timeout = 12

        with mock.patch('compose.cli.main.signals.signal', autospec=True) as mock_signal:
            attach_to_logs(project, log_printer, service_names, timeout)

        assert mock_signal.signal.mock_calls == [
            mock.call(mock_signal.SIGINT, mock.ANY),
            mock.call(mock_signal.SIGTERM, mock.ANY),
        ]
        log_printer.run.assert_called_once_with()


class SetupConsoleHandlerTestCase(unittest.TestCase):

    def setUp(self):
        self.stream = mock.Mock()
        self.stream.isatty.return_value = True
        self.handler = logging.StreamHandler(stream=self.stream)

    def test_with_tty_verbose(self):
        setup_console_handler(self.handler, True)
        assert type(self.handler.formatter) == ConsoleWarningFormatter
        assert '%(name)s' in self.handler.formatter._fmt
        assert '%(funcName)s' in self.handler.formatter._fmt

    def test_with_tty_not_verbose(self):
        setup_console_handler(self.handler, False)
        assert type(self.handler.formatter) == ConsoleWarningFormatter
        assert '%(name)s' not in self.handler.formatter._fmt
        assert '%(funcName)s' not in self.handler.formatter._fmt

    def test_with_not_a_tty(self):
        self.stream.isatty.return_value = False
        setup_console_handler(self.handler, False)
        assert type(self.handler.formatter) == logging.Formatter


class ConvergeStrategyFromOptsTestCase(unittest.TestCase):

    def test_invalid_opts(self):
        options = {'--force-recreate': True, '--no-recreate': True}
        with self.assertRaises(UserError):
            convergence_strategy_from_opts(options)

    def test_always(self):
        options = {'--force-recreate': True, '--no-recreate': False}
        self.assertEqual(
            convergence_strategy_from_opts(options),
            ConvergenceStrategy.always
        )

    def test_never(self):
        options = {'--force-recreate': False, '--no-recreate': True}
        self.assertEqual(
            convergence_strategy_from_opts(options),
            ConvergenceStrategy.never
        )

    def test_changed(self):
        options = {'--force-recreate': False, '--no-recreate': False}
        self.assertEqual(
            convergence_strategy_from_opts(options),
            ConvergenceStrategy.changed
        )
