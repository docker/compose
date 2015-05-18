import mock

from compose import service, migration
from compose.project import Project
from .testcases import DockerClientTestCase


class ProjectTest(DockerClientTestCase):

    def test_migration_to_labels(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('composetest', [web, db], self.client)

        self.client.create_container(name='composetest_web_1', **web.options)
        self.client.create_container(name='composetest_db_1', **db.options)

        with mock.patch.object(service, 'log', autospec=True) as mock_log:
            self.assertEqual(project.containers(stopped=True), [])
            self.assertEqual(mock_log.warn.call_count, 2)

        migration.migrate_project_to_labels(project)
        self.assertEqual(len(project.containers(stopped=True)), 2)
