from __future__ import absolute_import

from compose import container
from compose.cli.log_printer import LogPrinter
from compose.cli.main import attach_to_logs
from compose.cli.main import build_log_printer
from compose.project import Project
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

    def test_attach_to_logs(self):
        project = mock.create_autospec(Project)
        log_printer = mock.create_autospec(LogPrinter, containers=[])
        service_names = ['web', 'db']
        timeout = 12

        with mock.patch('compose.cli.main.signal', autospec=True) as mock_signal:
            attach_to_logs(project, log_printer, service_names, timeout)

        mock_signal.signal.assert_called_once_with(mock_signal.SIGINT, mock.ANY)
        log_printer.run.assert_called_once_with()
        project.stop.assert_called_once_with(
            service_names=service_names,
            timeout=timeout)
