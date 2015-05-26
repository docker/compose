from compose import legacy
from compose.project import Project
from .testcases import DockerClientTestCase


class ProjectTest(DockerClientTestCase):

    def setUp(self):
        super(ProjectTest, self).setUp()

        self.services = [
            self.create_service('web'),
            self.create_service('db'),
        ]

        self.project = Project('composetest', self.services, self.client)

        # Create a legacy container for each service
        for service in self.services:
            service.ensure_image_exists()
            self.client.create_container(
                name='{}_{}_1'.format(self.project.name, service.name),
                **service.options
            )

        # Create a single one-off legacy container
        self.client.create_container(
            name='{}_{}_run_1'.format(self.project.name, self.services[0].name),
            **self.services[0].options
        )

    def get_names(self, **kwargs):
        if 'stopped' not in kwargs:
            kwargs['stopped'] = True

        return list(legacy.get_legacy_container_names(
            self.client,
            self.project.name,
            [s.name for s in self.services],
            **kwargs
        ))

    def test_get_legacy_container_names(self):
        self.assertEqual(len(self.get_names()), len(self.services))

    def test_get_legacy_container_names_one_off(self):
        self.assertEqual(len(self.get_names(one_off=True)), 1)

    def test_migration_to_labels(self):
        with self.assertRaises(legacy.LegacyContainersError) as cm:
            self.assertEqual(self.project.containers(stopped=True), [])

        self.assertEqual(
            set(cm.exception.names),
            set(['composetest_web_1', 'composetest_db_1']),
        )

        legacy.migrate_project_to_labels(self.project)
        self.assertEqual(len(self.project.containers(stopped=True)), len(self.services))
