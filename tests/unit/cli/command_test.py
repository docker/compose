# ~*~ encoding: utf-8 ~*~
from __future__ import absolute_import
from __future__ import unicode_literals

import os

import pytest
import six

from compose.cli.command import get_config_path_from_options
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

    def test_multiple_path_from_env_custom_separator(self):
        with mock.patch.dict(os.environ):
            os.environ['COMPOSE_PATH_SEPARATOR'] = '^'
            os.environ['COMPOSE_FILE'] = 'c:\\one.yml^.\\semi;colon.yml'
            environment = Environment.from_env_file('.')
            assert get_config_path_from_options(
                '.', {}, environment
            ) == ['c:\\one.yml', '.\\semi;colon.yml']

    def test_no_path(self):
        environment = Environment.from_env_file('.')
        assert not get_config_path_from_options('.', {}, environment)

    def test_unicode_path_from_options(self):
        paths = [b'\xe5\xb0\xb1\xe5\x90\x83\xe9\xa5\xad/docker-compose.yml']
        opts = {'--file': paths}
        environment = Environment.from_env_file('.')
        assert get_config_path_from_options(
            '.', opts, environment
        ) == ['就吃饭/docker-compose.yml']

    @pytest.mark.skipif(six.PY3, reason='Env values in Python 3 are already Unicode')
    def test_unicode_path_from_env(self):
        with mock.patch.dict(os.environ):
            os.environ['COMPOSE_FILE'] = b'\xe5\xb0\xb1\xe5\x90\x83\xe9\xa5\xad/docker-compose.yml'
            environment = Environment.from_env_file('.')
            assert get_config_path_from_options(
                '.', {}, environment
            ) == ['就吃饭/docker-compose.yml']
