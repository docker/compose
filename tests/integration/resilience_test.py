from __future__ import absolute_import
from __future__ import unicode_literals

from .. import mock
from .testcases import DockerClientTestCase
from compose.project import Project


class ResilienceTest(DockerClientTestCase):
    def setUp(self):
        self.db = self.create_service('db', volumes=['/var/db'], command='top')
        self.project = Project('composetest', [self.db], self.client)

        container = self.db.create_container()
        self.db.start_container(container)
        self.host_path = container.get('Volumes')['/var/db']

    def test_successful_recreate(self):
        self.project.up(force_recreate=True)
        container = self.db.containers()[0]
        self.assertEqual(container.get('Volumes')['/var/db'], self.host_path)

    def test_create_failure(self):
        with mock.patch('compose.service.Service.create_container', crash):
            with self.assertRaises(Crash):
                self.project.up(force_recreate=True)

        self.project.up()
        container = self.db.containers()[0]
        self.assertEqual(container.get('Volumes')['/var/db'], self.host_path)

    def test_start_failure(self):
        with mock.patch('compose.service.Service.start_container', crash):
            with self.assertRaises(Crash):
                self.project.up(force_recreate=True)

        self.project.up()
        container = self.db.containers()[0]
        self.assertEqual(container.get('Volumes')['/var/db'], self.host_path)


class Crash(Exception):
    pass


def crash(*args, **kwargs):
    raise Crash()
