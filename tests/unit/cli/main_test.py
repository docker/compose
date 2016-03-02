from __future__ import absolute_import
from __future__ import unicode_literals

import logging

import pytest

from compose import container
from compose.cli.errors import UserError
from compose.cli.formatter import ConsoleWarningFormatter
from compose.cli.main import convergence_strategy_from_opts
from compose.cli.main import filter_containers_to_service_names
from compose.cli.main import setup_console_handler
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

    def test_filter_containers_to_service_names(self):
        containers = [
            mock_container('web', 1),
            mock_container('web', 2),
            mock_container('db', 1),
            mock_container('other', 1),
            mock_container('another', 1),
        ]
        service_names = ['web', 'db']
        actual = filter_containers_to_service_names(containers, service_names)
        assert actual == containers[:3]

    def test_filter_containers_to_service_names_all(self):
        containers = [
            mock_container('web', 1),
            mock_container('db', 1),
            mock_container('other', 1),
        ]
        service_names = []
        actual = filter_containers_to_service_names(containers, service_names)
        assert actual == containers


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
