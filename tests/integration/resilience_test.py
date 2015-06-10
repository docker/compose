from __future__ import unicode_literals
from __future__ import absolute_import

import mock

from compose.project import Project
from .testcases import DockerClientTestCase


class ResilienceTest(DockerClientTestCase):
    def test_recreate_fails(self):
        db = self.create_service('db', volumes=['/var/db'], command='top')
        project = Project('composetest', [db], self.client)

        container = db.create_container()
        db.start_container(container)
        host_path = container.get('Volumes')['/var/db']

        project.up()
        container = db.containers()[0]
        self.assertEqual(container.get('Volumes')['/var/db'], host_path)

        with mock.patch('compose.service.Service.create_container', crash):
            with self.assertRaises(Crash):
                project.up()

        project.up()
        container = db.containers()[0]
        self.assertEqual(container.get('Volumes')['/var/db'], host_path)


class Crash(Exception):
    pass


def crash(*args, **kwargs):
    raise Crash()
