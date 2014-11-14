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
