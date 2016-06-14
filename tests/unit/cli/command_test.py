from __future__ import absolute_import
from __future__ import unicode_literals

import os
import ssl

import pytest

from compose.cli.command import get_config_path_from_options
from compose.cli.command import get_tls_version
from compose.config.environment import Environment
from compose.const import IS_WINDOWS_PLATFORM
from tests import mock


class TestGetConfigPathFromOptions(object):

    def test_path_from_options(self):
        paths = ['one.yml', 'two.yml']
        opts = {'--file': paths}
        environment = Environment.from_env_file('.')
        assert get_config_path_from_options('.', opts, environment) == paths

    def test_single_path_from_env(self):
        with mock.patch.dict(os.environ):
            os.environ['COMPOSE_FILE'] = 'one.yml'
            environment = Environment.from_env_file('.')
            assert get_config_path_from_options('.', {}, environment) == ['one.yml']

    @pytest.mark.skipif(IS_WINDOWS_PLATFORM, reason='posix separator')
    def test_multiple_path_from_env(self):
        with mock.patch.dict(os.environ):
            os.environ['COMPOSE_FILE'] = 'one.yml:two.yml'
            environment = Environment.from_env_file('.')
            assert get_config_path_from_options(
                '.', {}, environment
            ) == ['one.yml', 'two.yml']

    @pytest.mark.skipif(not IS_WINDOWS_PLATFORM, reason='windows separator')
    def test_multiple_path_from_env_windows(self):
        with mock.patch.dict(os.environ):
            os.environ['COMPOSE_FILE'] = 'one.yml;two.yml'
            environment = Environment.from_env_file('.')
            assert get_config_path_from_options(
                '.', {}, environment
            ) == ['one.yml', 'two.yml']

    def test_no_path(self):
        environment = Environment.from_env_file('.')
        assert not get_config_path_from_options('.', {}, environment)


class TestGetTlsVersion(object):
    def test_get_tls_version_default(self):
        environment = {}
        assert get_tls_version(environment) is None

    @pytest.mark.skipif(not hasattr(ssl, 'PROTOCOL_TLSv1_2'), reason='TLS v1.2 unsupported')
    def test_get_tls_version_upgrade(self):
        environment = {'COMPOSE_TLS_VERSION': 'TLSv1_2'}
        assert get_tls_version(environment) == ssl.PROTOCOL_TLSv1_2

    def test_get_tls_version_unavailable(self):
        environment = {'COMPOSE_TLS_VERSION': 'TLSv5_5'}
        with mock.patch('compose.cli.command.log') as mock_log:
            tls_version = get_tls_version(environment)
        mock_log.warn.assert_called_once_with(mock.ANY)
        assert tls_version is None
