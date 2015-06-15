from docker.errors import APIError

from compose import legacy
from compose.project import Project
from .testcases import DockerClientTestCase


class LegacyTestCase(DockerClientTestCase):

    def setUp(self):
        super(LegacyTestCase, self).setUp()
        self.containers = []

        db = self.create_service('db')
        web = self.create_service('web', links=[(db, 'db')])
        nginx = self.create_service('nginx', links=[(web, 'web')])

        self.services = [db, web, nginx]
        self.project = Project('composetest', self.services, self.client)

        # Create a legacy container for each service
        for service in self.services:
            service.ensure_image_exists()
            container = self.client.create_container(
                name='{}_{}_1'.format(self.project.name, service.name),
                **service.options
            )
            self.client.start(container)
            self.containers.append(container)

        # Create a single one-off legacy container
        self.containers.append(self.client.create_container(
            name='{}_{}_run_1'.format(self.project.name, self.services[0].name),
            **self.services[0].options
        ))

    def tearDown(self):
        super(LegacyTestCase, self).tearDown()
        for container in self.containers:
            try:
                self.client.kill(container)
            except APIError:
                pass
            try:
                self.client.remove_container(container)
            except APIError:
                pass

    def get_legacy_containers(self, **kwargs):
        return list(legacy.get_legacy_containers(
            self.client,
            self.project.name,
            [s.name for s in self.services],
            **kwargs
        ))

    def test_get_legacy_container_names(self):
        self.assertEqual(len(self.get_legacy_containers()), len(self.services))

    def test_get_legacy_container_names_one_off(self):
        self.assertEqual(len(self.get_legacy_containers(stopped=True, one_off=True)), 1)

    def test_migration_to_labels(self):
        with self.assertRaises(legacy.LegacyContainersError) as cm:
            self.assertEqual(self.project.containers(stopped=True), [])

        self.assertEqual(
            set(cm.exception.names),
            set(['composetest_db_1', 'composetest_web_1', 'composetest_nginx_1']),
        )

        legacy.migrate_project_to_labels(self.project)
        self.assertEqual(len(self.project.containers(stopped=True)), len(self.services))
