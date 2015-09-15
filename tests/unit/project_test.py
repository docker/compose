from __future__ import unicode_literals
from .. import unittest
from compose.service import Service
from compose.project import Project
from compose.container import Container
from compose.const import LABEL_SERVICE

import mock
import docker


class ProjectTest(unittest.TestCase):
    def setUp(self):
        self.mock_client = mock.create_autospec(docker.Client)

    def test_from_dict(self):
        project = Project.from_dicts('composetest', [
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
        project = Project.from_dicts('composetest', [
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
        dicts = [
            {
                'name': 'web',
                'image': 'busybox:latest',
            },
            {
                'name': 'db',
                'image': 'busybox:latest',
            },
        ]
        project = Project.from_dicts('composetest', dicts, None)
        self.assertEqual(len(project.services), 2)
        self.assertEqual(project.get_service('web').name, 'web')
        self.assertEqual(project.get_service('web').options['image'], 'busybox:latest')
        self.assertEqual(project.get_service('db').name, 'db')
        self.assertEqual(project.get_service('db').options['image'], 'busybox:latest')

    def test_get_service(self):
        web = Service(
            project='composetest',
            name='web',
            client=None,
            image="busybox:latest",
        )
        project = Project('test', [web], None)
        self.assertEqual(project.get_service('web'), web)

    def test_get_services_returns_all_services_without_args(self):
        web = Service(
            project='composetest',
            name='web',
            image='foo',
        )
        console = Service(
            project='composetest',
            name='console',
            image='foo',
        )
        project = Project('test', [web, console], None)
        self.assertEqual(project.get_services(), [web, console])

    def test_get_services_returns_listed_services_with_args(self):
        web = Service(
            project='composetest',
            name='web',
            image='foo',
        )
        console = Service(
            project='composetest',
            name='console',
            image='foo',
        )
        project = Project('test', [web, console], None)
        self.assertEqual(project.get_services(['console']), [console])

    def test_get_services_with_include_links(self):
        db = Service(
            project='composetest',
            name='db',
            image='foo',
        )
        web = Service(
            project='composetest',
            name='web',
            image='foo',
            links=[(db, 'database')]
        )
        cache = Service(
            project='composetest',
            name='cache',
            image='foo'
        )
        console = Service(
            project='composetest',
            name='console',
            image='foo',
            links=[(web, 'web')]
        )
        project = Project('test', [web, db, cache, console], None)
        self.assertEqual(
            project.get_services(['console'], include_deps=True),
            [db, web, console]
        )

    def test_get_services_removes_duplicates_following_links(self):
        db = Service(
            project='composetest',
            name='db',
            image='foo',
        )
        web = Service(
            project='composetest',
            name='web',
            image='foo',
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
        self.mock_client.inspect_container.return_value = container_dict
        project = Project.from_dicts('test', [
            {
                'name': 'test',
                'image': 'busybox:latest',
                'volumes_from': ['aaa']
            }
        ], self.mock_client)
        self.assertEqual(project.get_service('test')._get_volumes_from(), [container_id])

    def test_use_volumes_from_service_no_container(self):
        container_name = 'test_vol_1'
        self.mock_client.containers.return_value = [
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
        ], self.mock_client)
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

    def test_net_unset(self):
        project = Project.from_dicts('test', [
            {
                'name': 'test',
                'image': 'busybox:latest',
            }
        ], self.mock_client)
        service = project.get_service('test')
        self.assertEqual(service.net.id, None)
        self.assertNotIn('NetworkMode', service._get_container_host_config({}))

    def test_use_net_from_container(self):
        container_id = 'aabbccddee'
        container_dict = dict(Name='aaa', Id=container_id)
        self.mock_client.inspect_container.return_value = container_dict
        project = Project.from_dicts('test', [
            {
                'name': 'test',
                'image': 'busybox:latest',
                'net': 'container:aaa'
            }
        ], self.mock_client)
        service = project.get_service('test')
        self.assertEqual(service.net.mode, 'container:' + container_id)

    def test_use_net_from_service(self):
        container_name = 'test_aaa_1'
        self.mock_client.containers.return_value = [
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
        ], self.mock_client)

        service = project.get_service('test')
        self.assertEqual(service.net.mode, 'container:' + container_name)

    def test_container_without_name(self):
        self.mock_client.containers.return_value = [
            {'Image': 'busybox:latest', 'Id': '1', 'Name': '1'},
            {'Image': 'busybox:latest', 'Id': '2', 'Name': None},
            {'Image': 'busybox:latest', 'Id': '3'},
        ]
        self.mock_client.inspect_container.return_value = {
            'Id': '1',
            'Config': {
                'Labels': {
                    LABEL_SERVICE: 'web',
                },
            },
        }
        project = Project.from_dicts(
            'test',
            [{
                'name': 'web',
                'image': 'busybox:latest',
            }],
            self.mock_client,
        )
        self.assertEqual([c.id for c in project.containers()], ['1'])
