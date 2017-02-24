from __future__ import absolute_import
from __future__ import unicode_literals

import pytest

from .. import unittest
from compose.config import ConfigurationError
from compose.network import check_remote_network_config
from compose.network import Network


class NetworkTest(unittest.TestCase):
    def test_check_remote_network_config_success(self):
        options = {'com.docker.network.driver.foo': 'bar'}
        net = Network(
            None, 'compose_test', 'net1', 'bridge',
            options
        )
        check_remote_network_config(
            {'Driver': 'bridge', 'Options': options}, net
        )

    def test_check_remote_network_config_whitelist(self):
        options = {'com.docker.network.driver.foo': 'bar'}
        remote_options = {
            'com.docker.network.driver.overlay.vxlanid_list': '257',
            'com.docker.network.driver.foo': 'bar'
        }
        net = Network(
            None, 'compose_test', 'net1', 'overlay',
            options
        )
        check_remote_network_config(
            {'Driver': 'overlay', 'Options': remote_options}, net
        )

    def test_check_remote_network_config_driver_mismatch(self):
        net = Network(None, 'compose_test', 'net1', 'overlay')
        with pytest.raises(ConfigurationError):
            check_remote_network_config(
                {'Driver': 'bridge', 'Options': {}}, net
            )

    def test_check_remote_network_config_options_mismatch(self):
        net = Network(None, 'compose_test', 'net1', 'overlay')
        with pytest.raises(ConfigurationError):
            check_remote_network_config({'Driver': 'overlay', 'Options': {
                'com.docker.network.driver.foo': 'baz'
            }}, net)

    def test_check_remote_network_config_null_remote(self):
        net = Network(None, 'compose_test', 'net1', 'overlay')
        check_remote_network_config(
            {'Driver': 'overlay', 'Options': None}, net
        )
