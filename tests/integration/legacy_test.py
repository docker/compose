import unittest

from docker.errors import APIError

from .. import mock
from .testcases import DockerClientTestCase
from compose import legacy
from compose.project import Project


class UtilitiesTestCase(unittest.TestCase):
    def test_has_container(self):
        self.assertTrue(
            legacy.has_container("composetest", "web", "composetest_web_1", one_off=False),
        )
        self.assertFalse(
            legacy.has_container("composetest", "web", "composetest_web_run_1", one_off=False),
        )

    def test_has_container_one_off(self):
        self.assertFalse(
            legacy.has_container("composetest", "web", "composetest_web_1", one_off=True),
        )
        self.assertTrue(
            legacy.has_container("composetest", "web", "composetest_web_run_1", one_off=True),
        )

    def test_has_container_different_project(self):
        self.assertFalse(
            legacy.has_container("composetest", "web", "otherapp_web_1", one_off=False),
        )
        self.assertFalse(
            legacy.has_container("composetest", "web", "otherapp_web_run_1", one_off=True),
        )

    def test_has_container_different_service(self):
        self.assertFalse(
            legacy.has_container("composetest", "web", "composetest_db_1", one_off=False),
        )
        self.assertFalse(
            legacy.has_container("composetest", "web", "composetest_db_run_1", one_off=True),
        )

    def test_is_valid_name(self):
        self.assertTrue(
            legacy.is_valid_name("composetest_web_1", one_off=False),
        )
        self.assertFalse(
            legacy.is_valid_name("composetest_web_run_1", one_off=False),
        )

    def test_is_valid_name_one_off(self):
        self.assertFalse(
            legacy.is_valid_name("composetest_web_1", one_off=True),
        )
        self.assertTrue(
            legacy.is_valid_name("composetest_web_run_1", one_off=True),
        )

    def test_is_valid_name_invalid(self):
        self.assertFalse(
            legacy.is_valid_name("foo"),
        )
        self.assertFalse(
            legacy.is_valid_name("composetest_web_lol_1", one_off=True),
        )

    def test_get_legacy_containers(self):
        client = mock.Mock()
        client.containers.return_value = [
            {
                "Id": "abc123",
                "Image": "def456",
                "Name": "composetest_web_1",
                "Labels": None,
            },
            {
                "Id": "ghi789",
                "Image": "def456",
                "Name": None,
                "Labels": None,
            },
            {
                "Id": "jkl012",
                "Image": "def456",
                "Labels": None,
            },
        ]

        containers = legacy.get_legacy_containers(client, "composetest", ["web"])

        self.assertEqual(len(containers), 1)
        self.assertEqual(containers[0].id, 'abc123')


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
            name='{}_{}_run_1'.format(self.project.name, db.name),
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
        return legacy.get_legacy_containers(
            self.client,
            self.project.name,
            [s.name for s in self.services],
            **kwargs
        )

    def test_get_legacy_container_names(self):
        self.assertEqual(len(self.get_legacy_containers()), len(self.services))

    def test_get_legacy_container_names_one_off(self):
        self.assertEqual(len(self.get_legacy_containers(one_off=True)), 1)

    def test_migration_to_labels(self):
        # Trying to get the container list raises an exception

        with self.assertRaises(legacy.LegacyContainersError) as cm:
            self.project.containers(stopped=True)

        self.assertEqual(
            set(cm.exception.names),
            set(['composetest_db_1', 'composetest_web_1', 'composetest_nginx_1']),
        )

        self.assertEqual(
            set(cm.exception.one_off_names),
            set(['composetest_db_run_1']),
        )

        # Migrate the containers

        legacy.migrate_project_to_labels(self.project)

        # Getting the list no longer raises an exception

        containers = self.project.containers(stopped=True)
        self.assertEqual(len(containers), len(self.services))

    def test_migration_one_off(self):
        # We've already migrated

        legacy.migrate_project_to_labels(self.project)

        # Trying to create a one-off container results in a Docker API error

        with self.assertRaises(APIError) as cm:
            self.project.get_service('db').create_container(one_off=True)

        # Checking for legacy one-off containers raises an exception

        with self.assertRaises(legacy.LegacyOneOffContainersError) as cm:
            legacy.check_for_legacy_containers(
                self.client,
                self.project.name,
                ['db'],
                allow_one_off=False,
            )

        self.assertEqual(
            set(cm.exception.one_off_names),
            set(['composetest_db_run_1']),
        )

        # Remove the old one-off container

        c = self.client.inspect_container('composetest_db_run_1')
        self.client.remove_container(c)

        # Checking no longer raises an exception

        legacy.check_for_legacy_containers(
            self.client,
            self.project.name,
            ['db'],
            allow_one_off=False,
        )

        # Creating a one-off container no longer results in an API error

        self.project.get_service('db').create_container(one_off=True)
        self.assertIsInstance(self.client.inspect_container('composetest_db_run_1'), dict)
