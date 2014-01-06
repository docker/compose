from __future__ import unicode_literals
from .testcases import DockerClientTestCase
from fig.container import Container

class ContainerTest(DockerClientTestCase):
    def test_from_ps(self):
        container = Container.from_ps(self.client, {
            "Id":"abc",
            "Image":"ubuntu:12.04",
            "Command":"sleep 300",
            "Created":1387384730,
            "Status":"Up 8 seconds",
            "Ports":None,
            "SizeRw":0,
            "SizeRootFs":0,
            "Names":["/db_1"]
        }, has_been_inspected=True)
        self.assertEqual(container.dictionary, {
            "ID": "abc",
            "Image":"ubuntu:12.04",
            "Name": "/db_1",
        })

    def test_environment(self):
        container = Container(self.client, {
            'ID': 'abc',
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
