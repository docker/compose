import mock

from compose import legacy
from compose.project import Project
from .testcases import DockerClientTestCase


class ProjectTest(DockerClientTestCase):

    def test_migration_to_labels(self):
        services = [
            self.create_service('web'),
            self.create_service('db'),
        ]

        project = Project('composetest', services, self.client)

        for service in services:
            service.ensure_image_exists()
            self.client.create_container(
                name='{}_{}_1'.format(project.name, service.name),
                **service.options
            )

        with mock.patch.object(legacy, 'log', autospec=True) as mock_log:
            self.assertEqual(project.containers(stopped=True), [])
            self.assertEqual(mock_log.warn.call_count, 2)

        legacy.migrate_project_to_labels(project)
        self.assertEqual(len(project.containers(stopped=True)), 2)
