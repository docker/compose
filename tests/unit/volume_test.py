from __future__ import absolute_import
from __future__ import unicode_literals

import docker
import pytest

from compose import volume
from tests import mock


@pytest.fixture
def mock_client():
    return mock.create_autospec(docker.APIClient)


class TestVolume(object):

    def test_remove_local_volume(self, mock_client):
        vol = volume.Volume(mock_client, 'foo', 'project')
        vol.remove()
        mock_client.remove_volume.assert_called_once_with('foo_project')

    def test_remove_external_volume(self, mock_client):
        vol = volume.Volume(mock_client, 'foo', 'project', external=True)
        vol.remove()
        assert not mock_client.remove_volume.called
