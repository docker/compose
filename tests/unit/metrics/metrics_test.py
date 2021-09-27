import unittest

from compose.metrics.client import MetricsCommand
from compose.metrics.client import Status


class MetricsTest(unittest.TestCase):
    @classmethod
    def test_metrics(cls):
        assert MetricsCommand('up', 'moby').to_map() == {
            'command': 'compose up',
            'context': 'moby',
            'status': 'success',
            'source': 'docker-compose',
        }

        assert MetricsCommand('down', 'local').to_map() == {
            'command': 'compose down',
            'context': 'local',
            'status': 'success',
            'source': 'docker-compose',
        }

        assert MetricsCommand('help', 'aci', Status.FAILURE).to_map() == {
            'command': 'compose help',
            'context': 'aci',
            'status': 'failure',
            'source': 'docker-compose',
        }

        assert MetricsCommand('run', 'ecs').to_map() == {
            'command': 'compose run',
            'context': 'ecs',
            'status': 'success',
            'source': 'docker-compose',
        }
