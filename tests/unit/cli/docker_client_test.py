from __future__ import unicode_literals
from __future__ import absolute_import
import os

import mock
from tests import unittest

from fig.cli import docker_client 


class DockerClientTestCase(unittest.TestCase):

    def test_docker_client_no_home(self):
        with mock.patch.dict(os.environ):
            del os.environ['HOME']
            docker_client.docker_client()

    def test_docker_client_socket(self):
        with mock.patch.dict(os.environ):
            os.environ['DOCKER_HOST'] = ''
            os.environ['DOCKER_TLS_VERIFY'] = ''
            client = docker_client.docker_client()
            self.assertEqual(client.base_url, 'http+unix://var/run/docker.sock')

    def test_docker_client_implicit_http(self):
        with mock.patch.dict(os.environ):
            os.environ['DOCKER_HOST'] = '127.0.0.1:2376'
            os.environ['DOCKER_TLS_VERIFY'] = ''
            client = docker_client.docker_client()
            self.assertEqual(client.base_url, 'http://127.0.0.1:2376')

    def test_docker_client_implicit_https(self):
        with mock.patch.dict(os.environ):
            os.environ['DOCKER_HOST'] = '127.0.0.1:2376'
            os.environ['DOCKER_TLS_VERIFY'] = '1'
            os.environ['DOCKER_CERT_PATH'] = 'tests/fixtures/certs'
            client = docker_client.docker_client()
            self.assertEqual(client.base_url, 'https://127.0.0.1:2376')

    def test_docker_client_implicit_hostname_http(self):
        with mock.patch.dict(os.environ):
            os.environ['DOCKER_HOST'] = ':2376'
            os.environ['DOCKER_TLS_VERIFY'] = ''
            client = docker_client.docker_client()
            self.assertEqual(client.base_url, 'http://127.0.0.1:2376')
