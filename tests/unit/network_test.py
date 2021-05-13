import pytest

from .. import mock
from .. import unittest
from compose.network import check_remote_network_config
from compose.network import Network
from compose.network import NetworkConfigChangedError


class NetworkTest(unittest.TestCase):
    def test_check_remote_network_config_success(self):
        options = {'com.docker.network.driver.foo': 'bar'}
        ipam_config = {
            'driver': 'default',
            'config': [
                {'subnet': '172.0.0.1/16', },
                {
                    'subnet': '156.0.0.1/25',
                    'gateway': '156.0.0.1',
                    'aux_addresses': ['11.0.0.1', '24.25.26.27'],
                    'ip_range': '156.0.0.1-254'
                }
            ],
            'options': {
                'iface': 'eth0',
            }
        }
        labels = {
            'com.project.tests.istest': 'true',
            'com.project.sound.track': 'way out of here',
        }
        remote_labels = labels.copy()
        remote_labels.update({
            'com.docker.compose.project': 'compose_test',
            'com.docker.compose.network': 'net1',
        })
        net = Network(
            None, 'compose_test', 'net1', 'bridge',
            options, enable_ipv6=True, ipam=ipam_config,
            labels=labels
        )
        check_remote_network_config(
            {
                'Driver': 'bridge',
                'Options': options,
                'EnableIPv6': True,
                'Internal': False,
                'Attachable': True,
                'IPAM': {
                    'Driver': 'default',
                    'Config': [{
                        'Subnet': '156.0.0.1/25',
                        'Gateway': '156.0.0.1',
                        'AuxiliaryAddresses': ['24.25.26.27', '11.0.0.1'],
                        'IPRange': '156.0.0.1-254'
                    }, {
                        'Subnet': '172.0.0.1/16',
                        'Gateway': '172.0.0.1'
                    }],
                    'Options': {
                        'iface': 'eth0',
                    },
                },
                'Labels': remote_labels
            },
            net
        )

    def test_check_remote_network_config_whitelist(self):
        options = {'com.docker.network.driver.foo': 'bar'}
        remote_options = {
            'com.docker.network.driver.overlay.vxlanid_list': '257',
            'com.docker.network.driver.foo': 'bar',
            'com.docker.network.windowsshim.hnsid': 'aac3fd4887daaec1e3b',
        }
        net = Network(
            None, 'compose_test', 'net1', 'overlay',
            options
        )
        check_remote_network_config(
            {'Driver': 'overlay', 'Options': remote_options}, net
        )

    @mock.patch('compose.network.Network.true_name', lambda n: n.full_name)
    def test_check_remote_network_config_driver_mismatch(self):
        net = Network(None, 'compose_test', 'net1', 'overlay')
        with pytest.raises(NetworkConfigChangedError) as e:
            check_remote_network_config(
                {'Driver': 'bridge', 'Options': {}}, net
            )

        assert 'driver has changed' in str(e.value)

    @mock.patch('compose.network.Network.true_name', lambda n: n.full_name)
    def test_check_remote_network_config_options_mismatch(self):
        net = Network(None, 'compose_test', 'net1', 'overlay')
        with pytest.raises(NetworkConfigChangedError) as e:
            check_remote_network_config({'Driver': 'overlay', 'Options': {
                'com.docker.network.driver.foo': 'baz'
            }}, net)

        assert 'option "com.docker.network.driver.foo" has changed' in str(e.value)

    def test_check_remote_network_config_null_remote(self):
        net = Network(None, 'compose_test', 'net1', 'overlay')
        check_remote_network_config(
            {'Driver': 'overlay', 'Options': None}, net
        )

    def test_check_remote_network_config_null_remote_ipam_options(self):
        ipam_config = {
            'driver': 'default',
            'config': [
                {'subnet': '172.0.0.1/16', },
                {
                    'subnet': '156.0.0.1/25',
                    'gateway': '156.0.0.1',
                    'aux_addresses': ['11.0.0.1', '24.25.26.27'],
                    'ip_range': '156.0.0.1-254'
                }
            ]
        }
        net = Network(
            None, 'compose_test', 'net1', 'bridge', ipam=ipam_config,
        )

        check_remote_network_config(
            {
                'Driver': 'bridge',
                'Attachable': True,
                'IPAM': {
                    'Driver': 'default',
                    'Config': [{
                        'Subnet': '156.0.0.1/25',
                        'Gateway': '156.0.0.1',
                        'AuxiliaryAddresses': ['24.25.26.27', '11.0.0.1'],
                        'IPRange': '156.0.0.1-254'
                    }, {
                        'Subnet': '172.0.0.1/16',
                        'Gateway': '172.0.0.1'
                    }],
                    'Options': None
                },
            },
            net
        )

    @mock.patch('compose.network.Network.true_name', lambda n: n.full_name)
    def test_check_remote_network_labels_mismatch(self):
        net = Network(None, 'compose_test', 'net1', 'overlay', labels={
            'com.project.touhou.character': 'sakuya.izayoi'
        })
        remote = {
            'Driver': 'overlay',
            'Options': None,
            'Labels': {
                'com.docker.compose.network': 'net1',
                'com.docker.compose.project': 'compose_test',
                'com.project.touhou.character': 'marisa.kirisame',
            }
        }
        with mock.patch('compose.network.log') as mock_log:
            check_remote_network_config(remote, net)

        mock_log.warning.assert_called_once_with(mock.ANY)
        _, args, kwargs = mock_log.warning.mock_calls[0]
        assert 'label "com.project.touhou.character" has changed' in args[0]

    def test_remote_config_labels_none(self):
        remote = {'Labels': None}
        local = Network(None, 'test_project', 'test_network')
        check_remote_network_config(remote, local)
