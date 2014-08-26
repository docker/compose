from __future__ import unicode_literals
from mock import Mock
from .. import unittest
from fig.service import Service
from fig.project import Project, ConfigurationError


class ProjectTest(unittest.TestCase):
    def test_from_dict(self):
        project = Project.from_dicts('figtest', [
            {
                'name': 'web',
                'image': 'busybox:latest'
            },
            {
                'name': 'db',
                'image': 'busybox:latest'
            },
        ], None)
        self.assertEqual(len(project.services), 2)
        self.assertEqual(project.get_service('web').name, 'web')
        self.assertEqual(project.get_service('web').options['image'], 'busybox:latest')
        self.assertEqual(project.get_service('db').name, 'db')
        self.assertEqual(project.get_service('db').options['image'], 'busybox:latest')

    def test_from_dict_sorts_in_dependency_order(self):
        project = Project.from_dicts('figtest', [
            {
                'name': 'web',
                'image': 'busybox:latest',
                'links': ['db'],
            },
            {
                'name': 'db',
                'image': 'busybox:latest',
                'volumes_from': ['volume']
            },
            {
                'name': 'volume',
                'image': 'busybox:latest',
                'volumes': ['/tmp'],
            }
        ], None)

        self.assertEqual(project.services[0].name, 'volume')
        self.assertEqual(project.services[1].name, 'db')
        self.assertEqual(project.services[2].name, 'web')

    def test_from_config(self):
        project = Project.from_config('figtest', {
            'web': {
                'image': 'busybox:latest',
            },
            'db': {
                'image': 'busybox:latest',
            },
        }, None)
        self.assertEqual(len(project.services), 2)
        self.assertEqual(project.get_service('web').name, 'web')
        self.assertEqual(project.get_service('web').options['image'], 'busybox:latest')
        self.assertEqual(project.get_service('db').name, 'db')
        self.assertEqual(project.get_service('db').options['image'], 'busybox:latest')

    def test_from_config_throws_error_when_not_dict(self):
        with self.assertRaises(ConfigurationError):
            project = Project.from_config('figtest', {
                'web': 'busybox:latest',
            }, None)

    def test_get_service(self):
        web = Service(
            project='figtest',
            name='web',
            client=None,
            image="busybox:latest",
        )
        project = Project('test', [web], None)
        self.assertEqual(project.get_service('web'), web)

    def test_get_services_returns_all_services_without_args(self):
        web = Service(
            project='figtest',
            name='web',
        )
        console = Service(
            project='figtest',
            name='console',
        )
        project = Project('test', [web, console], None)
        self.assertEqual(project.get_services(), [web, console])

    def test_get_services_returns_listed_services_with_args(self):
        web = Service(
            project='figtest',
            name='web',
        )
        console = Service(
            project='figtest',
            name='console',
        )
        project = Project('test', [web, console], None)
        self.assertEqual(project.get_services(['console']), [console])

    def test_get_services_with_include_links(self):
        db = Service(
            project='figtest',
            name='db',
        )
        web = Service(
            project='figtest',
            name='web',
            links=[(db, 'database')]
        )
        cache = Service(
            project='figtest',
            name='cache'
        )
        console = Service(
            project='figtest',
            name='console',
            links=[(web, 'web')]
        )
        project = Project('test', [web, db, cache, console], None)
        self.assertEqual(
            project.get_services(['console'], include_links=True),
            [db, web, console]
        )

    def test_get_services_removes_duplicates_following_links(self):
        db = Service(
            project='figtest',
            name='db',
        )
        web = Service(
            project='figtest',
            name='web',
            links=[(db, 'database')]
        )
        project = Project('test', [web, db], None)
        self.assertEqual(
            project.get_services(['web', 'db'], include_links=True),
            [db, web]
        )

    def test_skip_images_already_built(self):
        test = Service(name='test')
        srv1 = Mock(spec=test, build_path='/srv/myapp/')
        srv2 = Mock(spec=test, build_path='/srv/myapp/')
        srv3 = Mock(spec=test, build_path='/srv/db/')
        services = [srv1, srv2, srv3]
        project = Project('test', services, None)

        project.build()

        srv1.build.assert_called_with(False)
        self.assertFalse(srv2.build.called)
        self.assertTrue(srv2.tag.called)
        srv3.build.assert_called_with(False)

    def test_skip_services_with_existing_images(self):
        test = Service(name='test')
        srv1 = Mock(spec=test, build_path='/srv/myapp/')
        srv2 = Mock(spec=test)
        srv2.can_be_built.return_value = False
        project = Project('test', [srv1, srv2], None)
        project.build()
        srv1.build.assert_called_with(False)
        self.assertFalse(srv2.build.called)
