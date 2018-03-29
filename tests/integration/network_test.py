from __future__ import absolute_import
from __future__ import unicode_literals

import pytest

from .testcases import DockerClientTestCase
from compose.config.errors import ConfigurationError
from compose.const import LABEL_NETWORK
from compose.const import LABEL_PROJECT
from compose.network import Network


class NetworkTest(DockerClientTestCase):
    def test_network_default_labels(self):
        net = Network(self.client, 'composetest', 'foonet')
        net.ensure()
        net_data = net.inspect()
        labels = net_data['Labels']
        assert labels[LABEL_NETWORK] == net.name
        assert labels[LABEL_PROJECT] == net.project

    def test_network_external_default_ensure(self):
        net = Network(
            self.client, 'composetest', 'foonet',
            external=True
        )

        with pytest.raises(ConfigurationError):
            net.ensure()

    def test_network_external_overlay_ensure(self):
        net = Network(
            self.client, 'composetest', 'foonet',
            driver='overlay', external=True
        )

        assert net.ensure() is None
