from __future__ import unicode_literals

import docker

from .. import mock
from .. import unittest
from compose.container import Container
from compose.container import get_container_name


class ContainerTest(unittest.TestCase):

    def setUp(self):
        self.container_dict = {
            "Id": "abc",
            "Image": "busybox:latest",
            "Command": "top",
            "Created": 1387384730,
            "Status": "Up 8 seconds",
            "Ports": None,
            "SizeRw": 0,
            "SizeRootFs": 0,
            "Names": ["/composetest_db_1", "/composetest_web_1/db"],
            "NetworkSettings": {
                "Ports": {},
            },
            "Config": {
                "Labels": {
                    "com.docker.compose.project": "composetest",
                    "com.docker.compose.service": "web",
                    "com.docker.compose.container-number": 7,
                },
            }
        }

    def test_from_ps(self):
        container = Container.from_ps(None,
                                      self.container_dict,
                                      has_been_inspected=True)
        self.assertEqual(
            container.dictionary,
            {
                "Id": "abc",
                "Image": "busybox:latest",
                "Name": "/composetest_db_1",
            })

    def test_from_ps_prefixed(self):
        self.container_dict['Names'] = ['/swarm-host-1' + n for n in self.container_dict['Names']]

        container = Container.from_ps(None,
                                      self.container_dict,
                                      has_been_inspected=True)
        self.assertEqual(container.dictionary, {
            "Id": "abc",
            "Image": "busybox:latest",
            "Name": "/composetest_db_1",
        })

    def test_environment(self):
        container = Container(None, {
            'Id': 'abc',
            'Config': {
                'Env': [
                    'FOO=BAR',
                    'BAZ=DOGE',
                ]
            }
        }, has_been_inspected=True)
        self.assertEqual(container.environment, {
            'FOO': 'BAR',
            'BAZ': 'DOGE',
        })

    def test_number(self):
        container = Container(None, self.container_dict, has_been_inspected=True)
        self.assertEqual(container.number, 7)

    def test_name(self):
        container = Container.from_ps(None,
                                      self.container_dict,
                                      has_been_inspected=True)
        self.assertEqual(container.name, "composetest_db_1")

    def test_name_without_project(self):
        container = Container(None, self.container_dict, has_been_inspected=True)
        self.assertEqual(container.name_without_project, "web_7")

    def test_inspect_if_not_inspected(self):
        mock_client = mock.create_autospec(docker.Client)
        container = Container(mock_client, dict(Id="the_id"))

        container.inspect_if_not_inspected()
        mock_client.inspect_container.assert_called_once_with("the_id")
        self.assertEqual(container.dictionary,
                         mock_client.inspect_container.return_value)
        self.assertTrue(container.has_been_inspected)

        container.inspect_if_not_inspected()
        self.assertEqual(mock_client.inspect_container.call_count, 1)

    def test_human_readable_ports_none(self):
        container = Container(None, self.container_dict, has_been_inspected=True)
        self.assertEqual(container.human_readable_ports, '')

    def test_human_readable_ports_public_and_private(self):
        self.container_dict['NetworkSettings']['Ports'].update({
            "45454/tcp": [{"HostIp": "0.0.0.0", "HostPort": "49197"}],
            "45453/tcp": [],
        })
        container = Container(None, self.container_dict, has_been_inspected=True)

        expected = "45453/tcp, 0.0.0.0:49197->45454/tcp"
        self.assertEqual(container.human_readable_ports, expected)

    def test_get_local_port(self):
        self.container_dict['NetworkSettings']['Ports'].update({
            "45454/tcp": [{"HostIp": "0.0.0.0", "HostPort": "49197"}],
        })
        container = Container(None, self.container_dict, has_been_inspected=True)

        self.assertEqual(
            container.get_local_port(45454, protocol='tcp'),
            '0.0.0.0:49197')

    def test_get(self):
        container = Container(None, {
            "Status": "Up 8 seconds",
            "HostConfig": {
                "VolumesFrom": ["volume_id"]
            },
        }, has_been_inspected=True)

        self.assertEqual(container.get('Status'), "Up 8 seconds")
        self.assertEqual(container.get('HostConfig.VolumesFrom'), ["volume_id"])
        self.assertEqual(container.get('Foo.Bar.DoesNotExist'), None)


class GetContainerNameTestCase(unittest.TestCase):

    def test_get_container_name(self):
        self.assertIsNone(get_container_name({}))
        self.assertEqual(get_container_name({'Name': 'myproject_db_1'}), 'myproject_db_1')
        self.assertEqual(get_container_name({'Names': ['/myproject_db_1', '/myproject_web_1/db']}), 'myproject_db_1')
        self.assertEqual(
            get_container_name({
                'Names': [
                    '/swarm-host-1/myproject_db_1',
                    '/swarm-host-1/myproject_web_1/db'
                ]
            }),
            'myproject_db_1'
        )
