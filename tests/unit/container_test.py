from __future__ import unicode_literals
from .. import unittest
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
