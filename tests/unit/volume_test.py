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
        vol = volume.Volume(mock_client, 'foo', 'project', external_name='data')
        vol.remove()
        assert not mock_client.remove_volume.called

    def test_full_name_volume(self, mock_client):
        vol = volume.Volume(mock_client, 'project', 'foo')
        assert vol.full_name == 'project_foo'

    def test_full_name_volume_with_actual_name(self, mock_client):
        vol = volume.Volume(mock_client, 'project', 'foo', actual_name='my-foo')
        assert vol.full_name == 'my-foo'

    def test_full_name_volume_with_external_name(self, mock_client):
        vol = volume.Volume(mock_client, 'project', 'foo', external_name='my-external-foo')
        assert vol.full_name == 'my-external-foo'
