from __future__ import unicode_literals
from .. import unittest

import mock
import docker

from fig.container import Container


class ContainerTest(unittest.TestCase):
    def test_from_ps(self):
        container = Container.from_ps(None, {
            "Id":"abc",
            "Image":"busybox:latest",
            "Command":"sleep 300",
            "Created":1387384730,
            "Status":"Up 8 seconds",
            "Ports":None,
            "SizeRw":0,
            "SizeRootFs":0,
            "Names":["/figtest_db_1"]
        }, has_been_inspected=True)
        self.assertEqual(container.dictionary, {
            "Id": "abc",
            "Image":"busybox:latest",
            "Name": "/figtest_db_1",
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
        container = Container.from_ps(None, {
            "Id":"abc",
            "Image":"busybox:latest",
            "Command":"sleep 300",
            "Created":1387384730,
            "Status":"Up 8 seconds",
            "Ports":None,
            "SizeRw":0,
            "SizeRootFs":0,
            "Names":["/figtest_db_1"]
        }, has_been_inspected=True)
        self.assertEqual(container.number, 1)

    def test_name(self):
        container = Container.from_ps(None, {
            "Id":"abc",
            "Image":"busybox:latest",
            "Command":"sleep 300",
            "Names":["/figtest_db_1"]
        }, has_been_inspected=True)
        self.assertEqual(container.name, "figtest_db_1")

    def test_name_without_project(self):
        container = Container.from_ps(None, {
            "Id":"abc",
            "Image":"busybox:latest",
            "Command":"sleep 300",
            "Names":["/figtest_db_1"]
        }, has_been_inspected=True)
        self.assertEqual(container.name_without_project, "db_1")

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
