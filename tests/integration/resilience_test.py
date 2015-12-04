from __future__ import absolute_import
from __future__ import unicode_literals

from .. import mock
from .testcases import DockerClientTestCase
from compose.config.types import VolumeSpec
from compose.project import Project
from compose.service import ConvergenceStrategy


class ResilienceTest(DockerClientTestCase):
    def setUp(self):
        self.db = self.create_service(
            'db',
            volumes=[VolumeSpec.parse('/var/db')],
            command='top')
        self.project = Project('composetest', [self.db], self.client)

        container = self.db.create_container()
        container.start()
        self.host_path = container.get('Volumes')['/var/db']

    def test_successful_recreate(self):
        self.project.up(strategy=ConvergenceStrategy.always)
        container = self.db.containers()[0]
        self.assertEqual(container.get('Volumes')['/var/db'], self.host_path)

    def test_create_failure(self):
        with mock.patch('compose.service.Service.create_container', crash):
            with self.assertRaises(Crash):
                self.project.up(strategy=ConvergenceStrategy.always)

        self.project.up()
        container = self.db.containers()[0]
        self.assertEqual(container.get('Volumes')['/var/db'], self.host_path)

    def test_start_failure(self):
        with mock.patch('compose.container.Container.start', crash):
            with self.assertRaises(Crash):
                self.project.up(strategy=ConvergenceStrategy.always)

        self.project.up()
        container = self.db.containers()[0]
        self.assertEqual(container.get('Volumes')['/var/db'], self.host_path)


class Crash(Exception):
    pass


def crash(*args, **kwargs):
    raise Crash()
