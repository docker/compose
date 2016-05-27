from __future__ import absolute_import
from __future__ import unicode_literals

from .. import mock
from .testcases import DockerClientTestCase
from compose.config.types import VolumeSpec
from compose.core.project import Project
from compose.core.service import ConvergenceStrategy

service_create_container_fqmn = 'compose.core.service.Service.create_container'


class ResilienceTest(DockerClientTestCase):
    def setUp(self):
        self.db = self.create_service(
            'db',
            volumes=[VolumeSpec.parse('/var/db')],
            command='top')
        self.project = Project('composetest', [self.db], self.client)

        container = self.db.create_container()
        self.db.start_container(container)
        self.host_path = container.get_mount('/var/db')['Source']

    def test_successful_recreate(self):
        self.project.up(strategy=ConvergenceStrategy.always)
        container = self.db.containers()[0]
        self.assertEqual(container.get_mount('/var/db')['Source'], self.host_path)

    def test_create_failure(self):
        with mock.patch(service_create_container_fqmn, crash):
            with self.assertRaises(Crash):
                self.project.up(strategy=ConvergenceStrategy.always)

        self.project.up()
        container = self.db.containers()[0]
        self.assertEqual(container.get_mount('/var/db')['Source'], self.host_path)

    def test_start_failure(self):
        with mock.patch(service_create_container_fqmn, crash):
            with self.assertRaises(Crash):
                self.project.up(strategy=ConvergenceStrategy.always)

        self.project.up()
        container = self.db.containers()[0]
        self.assertEqual(container.get_mount('/var/db')['Source'], self.host_path)


class Crash(Exception):
    pass


def crash(*args, **kwargs):
    raise Crash()
