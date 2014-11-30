from __future__ import unicode_literals
from .. import unittest
from fig.service import Service
from fig.container import Container
from fig.project import Project, ConfigurationError
import mock

import docker

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

    def test_get_services_with_include_links_service(self):
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
            project.get_services(['console'], include_deps=True),
            [db, web, console]
        )

    def test_get_services_with_include_links_project(self):
        project = Project.from_dicts('figtest', [
            {
                'name': 'db',
                'image': 'busybox:latest'
            },
            {
                'name': 'web',
                'image': 'busybox:latest',
                'links': ['db:database'],
            },
            {
                'name': 'cache',
                'image': 'busybox'
            },
            {
                'name': 'console',
                'image': 'busybox:latest',
                'links': ['web:web']
            }
        ], None)
        self.assertEqual(
            [s.name for s in project.get_services(['console'], include_deps=True)],
            [s for s in ['db', 'web', 'console']]
        )

    def test_get_services_removes_duplicates_following_links_project(self):
        project = Project.from_dicts('test', [
            {
                'name': 'db',
                'image': 'busybox:latest',
            },
            {
                'name': 'web',
                'image': 'busybox:latest',
                'links': ['db:database'],
            }
        ], None)
        self.assertEqual(
            [s.name for s in project.get_services(['web', 'db'], include_deps=True)],
            [s for s in ['db', 'web']]
        )

    def test_get_services_removes_duplicates_following_links_service(self):
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
            project.get_services(['web', 'db'], include_deps=True),
            [db, web]
        )

    def test_use_volumes_from_container(self):
        container_id = 'aabbccddee'
        container_dict = dict(Name='aaa', Id=container_id)
        mock_client = mock.create_autospec(docker.Client)
        mock_client.inspect_container.return_value = container_dict
        project = Project.from_dicts('test', [
            {
                'name': 'test',
                'image': 'busybox:latest',
                'volumes_from': ['aaa']
            }
        ], mock_client)

        self.assertEqual(project.get_service('test')._get_volumes_from(), [container_id])

    def test_use_volumes_from_service_no_container(self):
        container_name = 'test_vol_1'
        mock_client = mock.create_autospec(docker.Client)
        mock_client.containers.return_value = [
            {
                "Name": container_name,
                "Names": [container_name],
                "Id": container_name,
                "Image": 'busybox:latest'
            }
        ]
        project = Project.from_dicts('test', [
            {
                'name': 'vol',
                'image': 'busybox:latest'
            },
            {
                'name': 'test',
                'image': 'busybox:latest',
                'volumes_from': ['vol']
            }
        ], mock_client)

        self.assertEqual(project.get_service('test')._get_volumes_from(), [container_name])

    @mock.patch.object(Service, 'containers')
    def test_use_volumes_from_service_container(self, mock_return):
        container_ids = ['aabbccddee', '12345']
        mock_return.return_value = [
            mock.Mock(id=container_id, spec=Container)
            for container_id in container_ids]

        project = Project.from_dicts('test', [
            {
                'name': 'vol',
                'image': 'busybox:latest'
            },
            {
                'name': 'test',
                'image': 'busybox:latest',
                'volumes_from': ['vol']
            }
        ], None)

        self.assertEqual(project.get_service('test')._get_volumes_from(), container_ids)

    def test_use_net_from_container(self):
        container_id = 'aabbccddee'
        container_dict = dict(Name='aaa', Id=container_id)
        mock_client = mock.create_autospec(docker.Client)
        mock_client.inspect_container.return_value = container_dict
        project = Project.from_dicts('test', [
            {
                'name': 'test',
                'image': 'busybox:latest',
                'net': 'container:aaa'
            }
        ], mock_client)

        service = project.get_service('test')
        self.assertEqual(service._get_net(), 'container:'+container_id)

    def test_use_net_from_service(self):
        container_name = 'test_aaa_1'
        mock_client = mock.create_autospec(docker.Client)
        mock_client.containers.return_value = [
            {
                "Name": container_name,
                "Names": [container_name],
                "Id": container_name,
                "Image": 'busybox:latest'
            }
        ]
        project = Project.from_dicts('test', [
            {
                'name': 'aaa',
                'image': 'busybox:latest'
            },
            {
                'name': 'test',
                'image': 'busybox:latest',
                'net': 'container:aaa'
            }
        ], mock_client)

        service = project.get_service('test')
        self.assertEqual(service._get_net(), 'container:'+container_name)
