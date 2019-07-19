from __future__ import absolute_import
from __future__ import unicode_literals

import docker

from .. import mock
from .. import unittest
from ..helpers import BUSYBOX_IMAGE_WITH_TAG
from compose.const import LABEL_ONE_OFF
from compose.const import LABEL_SLUG
from compose.container import Container
from compose.container import get_container_name


class ContainerTest(unittest.TestCase):

    def setUp(self):
        self.container_id = "abcabcabcbabc12345"
        self.container_dict = {
            "Id": self.container_id,
            "Image": BUSYBOX_IMAGE_WITH_TAG,
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
                    "com.docker.compose.container-number": "7",
                },
            }
        }

    def test_from_ps(self):
        container = Container.from_ps(None,
                                      self.container_dict,
                                      has_been_inspected=True)
        assert container.dictionary == {
            "Id": self.container_id,
            "Image": BUSYBOX_IMAGE_WITH_TAG,
            "Name": "/composetest_db_1",
        }

    def test_from_ps_prefixed(self):
        self.container_dict['Names'] = [
            '/swarm-host-1' + n for n in self.container_dict['Names']
        ]

        container = Container.from_ps(
            None,
            self.container_dict,
            has_been_inspected=True)
        assert container.dictionary == {
            "Id": self.container_id,
            "Image": BUSYBOX_IMAGE_WITH_TAG,
            "Name": "/composetest_db_1",
        }

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
        assert container.environment == {
            'FOO': 'BAR',
            'BAZ': 'DOGE',
        }

    def test_number(self):
        container = Container(None, self.container_dict, has_been_inspected=True)
        assert container.number == 7

    def test_name(self):
        container = Container.from_ps(None,
                                      self.container_dict,
                                      has_been_inspected=True)
        assert container.name == "composetest_db_1"

    def test_name_without_project(self):
        self.container_dict['Name'] = "/composetest_web_7"
        container = Container(None, self.container_dict, has_been_inspected=True)
        assert container.name_without_project == "web_7"

    def test_name_without_project_custom_container_name(self):
        self.container_dict['Name'] = "/custom_name_of_container"
        container = Container(None, self.container_dict, has_been_inspected=True)
        assert container.name_without_project == "custom_name_of_container"

    def test_name_without_project_one_off(self):
        self.container_dict['Name'] = "/composetest_web_092cd63296f"
        self.container_dict['Config']['Labels'][LABEL_SLUG] = (
            "092cd63296fdc446ad432d3905dd1fcbe12a2ba6b52"
        )
        self.container_dict['Config']['Labels'][LABEL_ONE_OFF] = 'True'
        container = Container(None, self.container_dict, has_been_inspected=True)
        assert container.name_without_project == 'web_092cd63296fd'

    def test_inspect_if_not_inspected(self):
        mock_client = mock.create_autospec(docker.APIClient)
        container = Container(mock_client, dict(Id="the_id"))

        container.inspect_if_not_inspected()
        mock_client.inspect_container.assert_called_once_with("the_id")
        assert container.dictionary == mock_client.inspect_container.return_value
        assert container.has_been_inspected

        container.inspect_if_not_inspected()
        assert mock_client.inspect_container.call_count == 1

    def test_human_readable_ports_none(self):
        container = Container(None, self.container_dict, has_been_inspected=True)
        assert container.human_readable_ports == ''

    def test_human_readable_ports_public_and_private(self):
        self.container_dict['NetworkSettings']['Ports'].update({
            "45454/tcp": [{"HostIp": "0.0.0.0", "HostPort": "49197"}],
            "45453/tcp": [],
        })
        container = Container(None, self.container_dict, has_been_inspected=True)

        expected = "45453/tcp, 0.0.0.0:49197->45454/tcp"
        assert container.human_readable_ports == expected

    def test_get_local_port(self):
        self.container_dict['NetworkSettings']['Ports'].update({
            "45454/tcp": [{"HostIp": "0.0.0.0", "HostPort": "49197"}],
        })
        container = Container(None, self.container_dict, has_been_inspected=True)

        assert container.get_local_port(45454, protocol='tcp') == '0.0.0.0:49197'

    def test_human_readable_states_no_health(self):
        container = Container(None, {
            "State": {
                "Status": "running",
                "Running": True,
                "Paused": False,
                "Restarting": False,
                "OOMKilled": False,
                "Dead": False,
                "Pid": 7623,
                "ExitCode": 0,
                "Error": "",
                "StartedAt": "2018-01-29T00:34:25.2052414Z",
                "FinishedAt": "0001-01-01T00:00:00Z"
            },
        }, has_been_inspected=True)
        expected = "Up"
        assert container.human_readable_state == expected

    def test_human_readable_states_starting(self):
        container = Container(None, {
            "State": {
                "Status": "running",
                "Running": True,
                "Paused": False,
                "Restarting": False,
                "OOMKilled": False,
                "Dead": False,
                "Pid": 11744,
                "ExitCode": 0,
                "Error": "",
                "StartedAt": "2018-02-03T07:56:20.3591233Z",
                "FinishedAt": "2018-01-31T08:56:11.0505228Z",
                "Health": {
                    "Status": "starting",
                    "FailingStreak": 0,
                    "Log": []
                }
            }
        }, has_been_inspected=True)
        expected = "Up (health: starting)"
        assert container.human_readable_state == expected

    def test_human_readable_states_healthy(self):
        container = Container(None, {
            "State": {
                "Status": "running",
                "Running": True,
                "Paused": False,
                "Restarting": False,
                "OOMKilled": False,
                "Dead": False,
                "Pid": 5674,
                "ExitCode": 0,
                "Error": "",
                "StartedAt": "2018-02-03T08:32:05.3281831Z",
                "FinishedAt": "2018-02-03T08:11:35.7872706Z",
                "Health": {
                    "Status": "healthy",
                    "FailingStreak": 0,
                    "Log": []
                }
            }
        }, has_been_inspected=True)
        expected = "Up (healthy)"
        assert container.human_readable_state == expected

    def test_get(self):
        container = Container(None, {
            "Status": "Up 8 seconds",
            "HostConfig": {
                "VolumesFrom": ["volume_id"]
            },
        }, has_been_inspected=True)

        assert container.get('Status') == "Up 8 seconds"
        assert container.get('HostConfig.VolumesFrom') == ["volume_id"]
        assert container.get('Foo.Bar.DoesNotExist') is None

    def test_short_id(self):
        container = Container(None, self.container_dict, has_been_inspected=True)
        assert container.short_id == self.container_id[:12]

    def test_has_api_logs(self):
        container_dict = {
            'HostConfig': {
                'LogConfig': {
                    'Type': 'json-file'
                }
            }
        }

        container = Container(None, container_dict, has_been_inspected=True)
        assert container.has_api_logs is True

        container_dict['HostConfig']['LogConfig']['Type'] = 'none'
        container = Container(None, container_dict, has_been_inspected=True)
        assert container.has_api_logs is False

        container_dict['HostConfig']['LogConfig']['Type'] = 'syslog'
        container = Container(None, container_dict, has_been_inspected=True)
        assert container.has_api_logs is False

        container_dict['HostConfig']['LogConfig']['Type'] = 'journald'
        container = Container(None, container_dict, has_been_inspected=True)
        assert container.has_api_logs is True

        container_dict['HostConfig']['LogConfig']['Type'] = 'foobar'
        container = Container(None, container_dict, has_been_inspected=True)
        assert container.has_api_logs is False


class GetContainerNameTestCase(unittest.TestCase):

    def test_get_container_name(self):
        assert get_container_name({}) is None
        assert get_container_name({'Name': 'myproject_db_1'}) == 'myproject_db_1'
        assert get_container_name(
            {'Names': ['/myproject_db_1', '/myproject_web_1/db']}
        ) == 'myproject_db_1'
        assert get_container_name({
            'Names': [
                '/swarm-host-1/myproject_db_1',
                '/swarm-host-1/myproject_web_1/db'
            ]
        }) == 'myproject_db_1'
