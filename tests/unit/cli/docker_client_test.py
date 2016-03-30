from __future__ import absolute_import
from __future__ import unicode_literals

import os

import docker
import pytest

from compose.cli.docker_client import docker_client
from compose.cli.docker_client import tls_config_from_options
from tests import mock
from tests import unittest


class DockerClientTestCase(unittest.TestCase):

    def test_docker_client_no_home(self):
        with mock.patch.dict(os.environ):
            del os.environ['HOME']
            docker_client(os.environ)

    def test_docker_client_with_custom_timeout(self):
        timeout = 300
        with mock.patch('compose.cli.docker_client.HTTP_TIMEOUT', 300):
            client = docker_client(os.environ)
            self.assertEqual(client.timeout, int(timeout))


class TLSConfigTestCase(unittest.TestCase):
    ca_cert = 'tests/fixtures/tls/ca.pem'
    client_cert = 'tests/fixtures/tls/cert.pem'
    key = 'tests/fixtures/tls/key.key'

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
