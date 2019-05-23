from __future__ import absolute_import
from __future__ import unicode_literals

import os
import platform
import ssl

import docker
import pytest

import compose
from compose.cli import errors
from compose.cli.docker_client import docker_client
from compose.cli.docker_client import get_tls_version
from compose.cli.docker_client import tls_config_from_options
from compose.config.environment import Environment
from tests import mock
from tests import unittest


class DockerClientTestCase(unittest.TestCase):

    def test_docker_client_no_home(self):
        with mock.patch.dict(os.environ):
            try:
                del os.environ['HOME']
            except KeyError:
                pass
            docker_client(os.environ)

    @mock.patch.dict(os.environ)
    def test_docker_client_with_custom_timeout(self):
        os.environ['COMPOSE_HTTP_TIMEOUT'] = '123'
        client = docker_client(os.environ)
        assert client.timeout == 123

    @mock.patch.dict(os.environ)
    def test_custom_timeout_error(self):
        os.environ['COMPOSE_HTTP_TIMEOUT'] = '123'
        client = docker_client(os.environ)

        with mock.patch('compose.cli.errors.log') as fake_log:
            with pytest.raises(errors.ConnectionError):
                with errors.handle_connection_errors(client):
                    raise errors.RequestsConnectionError(
                        errors.ReadTimeoutError(None, None, None))

        assert fake_log.error.call_count == 1
        assert '123' in fake_log.error.call_args[0][0]

        with mock.patch('compose.cli.errors.log') as fake_log:
            with pytest.raises(errors.ConnectionError):
                with errors.handle_connection_errors(client):
                    raise errors.ReadTimeout()

        assert fake_log.error.call_count == 1
        assert '123' in fake_log.error.call_args[0][0]

    def test_user_agent(self):
        client = docker_client(os.environ)
        expected = "docker-compose/{0} docker-py/{1} {2}/{3}".format(
            compose.__version__,
            docker.__version__,
            platform.system(),
            platform.release()
        )
        assert client.headers['User-Agent'] == expected


class TLSConfigTestCase(unittest.TestCase):
    cert_path = 'tests/fixtures/tls/'
    ca_cert = os.path.join(cert_path, 'ca.pem')
    client_cert = os.path.join(cert_path, 'cert.pem')
    key = os.path.join(cert_path, 'key.pem')

    def test_simple_tls(self):
        options = {'--tls': True}
        result = tls_config_from_options(options)
        assert result is True

    def test_tls_ca_cert(self):
        options = {
            '--tlscacert': self.ca_cert, '--tlsverify': True
        }
        result = tls_config_from_options(options)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.ca_cert == options['--tlscacert']
        assert result.verify is True

    def test_tls_ca_cert_explicit(self):
        options = {
            '--tlscacert': self.ca_cert, '--tls': True,
            '--tlsverify': True
        }
        result = tls_config_from_options(options)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.ca_cert == options['--tlscacert']
        assert result.verify is True

    def test_tls_client_cert(self):
        options = {
            '--tlscert': self.client_cert, '--tlskey': self.key
        }
        result = tls_config_from_options(options)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.cert == (options['--tlscert'], options['--tlskey'])

    def test_tls_client_cert_explicit(self):
        options = {
            '--tlscert': self.client_cert, '--tlskey': self.key,
            '--tls': True
        }
        result = tls_config_from_options(options)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.cert == (options['--tlscert'], options['--tlskey'])

    def test_tls_client_and_ca(self):
        options = {
            '--tlscert': self.client_cert, '--tlskey': self.key,
            '--tlsverify': True, '--tlscacert': self.ca_cert
        }
        result = tls_config_from_options(options)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.cert == (options['--tlscert'], options['--tlskey'])
        assert result.ca_cert == options['--tlscacert']
        assert result.verify is True

    def test_tls_client_and_ca_explicit(self):
        options = {
            '--tlscert': self.client_cert, '--tlskey': self.key,
            '--tlsverify': True, '--tlscacert': self.ca_cert,
            '--tls': True
        }
        result = tls_config_from_options(options)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.cert == (options['--tlscert'], options['--tlskey'])
        assert result.ca_cert == options['--tlscacert']
        assert result.verify is True

    def test_tls_client_missing_key(self):
        options = {'--tlscert': self.client_cert}
        with pytest.raises(docker.errors.TLSParameterError):
            tls_config_from_options(options)

        options = {'--tlskey': self.key}
        with pytest.raises(docker.errors.TLSParameterError):
            tls_config_from_options(options)

    def test_assert_hostname_explicit_skip(self):
        options = {'--tlscacert': self.ca_cert, '--skip-hostname-check': True}
        result = tls_config_from_options(options)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.assert_hostname is False

    def test_tls_client_and_ca_quoted_paths(self):
        options = {
            '--tlscacert': '"{0}"'.format(self.ca_cert),
            '--tlscert': '"{0}"'.format(self.client_cert),
            '--tlskey': '"{0}"'.format(self.key),
            '--tlsverify': True
        }
        result = tls_config_from_options(options)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.cert == (self.client_cert, self.key)
        assert result.ca_cert == self.ca_cert
        assert result.verify is True

    def test_tls_simple_with_tls_version(self):
        tls_version = 'TLSv1'
        options = {'--tls': True}
        environment = Environment({'COMPOSE_TLS_VERSION': tls_version})
        result = tls_config_from_options(options, environment)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.ssl_version == ssl.PROTOCOL_TLSv1

    def test_tls_mixed_environment_and_flags(self):
        options = {'--tls': True, '--tlsverify': False}
        environment = Environment({'DOCKER_CERT_PATH': 'tests/fixtures/tls/'})
        result = tls_config_from_options(options, environment)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.cert == (self.client_cert, self.key)
        assert result.ca_cert == self.ca_cert
        assert result.verify is False

    def test_tls_flags_override_environment(self):
        environment = Environment({
            'DOCKER_CERT_PATH': '/completely/wrong/path',
            'DOCKER_TLS_VERIFY': 'false'
        })
        options = {
            '--tlscacert': '"{0}"'.format(self.ca_cert),
            '--tlscert': '"{0}"'.format(self.client_cert),
            '--tlskey': '"{0}"'.format(self.key),
            '--tlsverify': True
        }

        result = tls_config_from_options(options, environment)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.cert == (self.client_cert, self.key)
        assert result.ca_cert == self.ca_cert
        assert result.verify is True

    def test_tls_verify_flag_no_override(self):
        environment = Environment({
            'DOCKER_TLS_VERIFY': 'true',
            'COMPOSE_TLS_VERSION': 'TLSv1',
            'DOCKER_CERT_PATH': self.cert_path
        })
        options = {'--tls': True, '--tlsverify': False}

        result = tls_config_from_options(options, environment)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.ssl_version == ssl.PROTOCOL_TLSv1
        # verify is a special case - since `--tlsverify` = False means it
        # wasn't used, we set it if either the environment or the flag is True
        # see https://github.com/docker/compose/issues/5632
        assert result.verify is True

    def test_tls_verify_env_falsy_value(self):
        environment = Environment({'DOCKER_TLS_VERIFY': '0'})
        options = {'--tls': True}
        assert tls_config_from_options(options, environment) is True

    def test_tls_verify_default_cert_path(self):
        environment = Environment({'DOCKER_TLS_VERIFY': '1'})
        options = {'--tls': True}
        with mock.patch('compose.cli.docker_client.default_cert_path') as dcp:
            dcp.return_value = 'tests/fixtures/tls/'
            result = tls_config_from_options(options, environment)
        assert isinstance(result, docker.tls.TLSConfig)
        assert result.verify is True
        assert result.ca_cert == self.ca_cert
        assert result.cert == (self.client_cert, self.key)


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
        with mock.patch('compose.cli.docker_client.log') as mock_log:
            tls_version = get_tls_version(environment)
        mock_log.warning.assert_called_once_with(mock.ANY)
        assert tls_version is None
