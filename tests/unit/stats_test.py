from __future__ import absolute_import
from __future__ import unicode_literals

from .. import mock
from .. import unittest
from compose import stats
from compose.container import Container


class StatsTestCase(unittest.TestCase):
    def setUp(self):
        self.data = {
            "cpu_stats": {
                "cpu_usage": {
                    "total_usage": 5,
                    "percpu_usage": [1, 2, 3, 4]
                },
                "system_cpu_usage": 100
            },
            "precpu_stats": {
                "cpu_usage": {
                    "total_usage": 3
                },
                "system_cpu_usage": 90
            },
            "memory_stats": {
                "usage": 5,
                "limit": 10
            },
            "networks": {
                "eth0": {
                    "rx_bytes": 5338,
                    "rx_dropped": 0,
                    "rx_errors": 0,
                    "rx_packets": 36,
                    "tx_bytes": 648,
                    "tx_dropped": 0,
                    "tx_errors": 0,
                    "tx_packets": 8
                },
                "eth5": {
                    "rx_bytes": 4641,
                    "rx_dropped": 0,
                    "rx_errors": 0,
                    "rx_packets": 26,
                    "tx_bytes": 690,
                    "tx_dropped": 0,
                    "tx_errors": 0,
                    "tx_packets": 9
                }
            }
        }
        self.mock_container = mock.create_autospec(Container)
        self.stats = stats.Stats(self.mock_container, self.data)

    def test_calculate_cpu_percent_unix(self):
        self.assertEqual(self.stats.calculate_cpu_percent_unix(), 80.0)

    def test_calc_mem_usage(self):
        self.assertEqual(self.stats.calculate_mem_usage(), 5)

    def test_calc_mem_limit(self):
        self.assertEqual(self.stats.calculate_mem_limit(), 10)

    def test_calc_net_io(self):
        self.assertEqual(self.stats.calculate_net_io(), (9979, 1338))
