from __future__ import absolute_import
from __future__ import unicode_literals

from docker.errors import DockerException

from .testcases import DockerClientTestCase
from compose.const import LABEL_PROJECT
from compose.const import LABEL_VOLUME
from compose.volume import Volume


class VolumeTest(DockerClientTestCase):
    def setUp(self):
        self.tmp_volumes = []

    def tearDown(self):
        for volume in self.tmp_volumes:
            try:
                self.client.remove_volume(volume.full_name)
            except DockerException:
                pass
        del self.tmp_volumes
        super(VolumeTest, self).tearDown()

    def create_volume(self, name, driver=None, opts=None, external=None):
        if external and isinstance(external, bool):
            external = name
        vol = Volume(
            self.client, 'composetest', name, driver=driver, driver_opts=opts,
            external_name=external
        )
        self.tmp_volumes.append(vol)
        return vol

    def test_create_volume(self):
        vol = self.create_volume('volume01')
        vol.create()
        info = self.client.inspect_volume(vol.full_name)
        assert info['Name'] == vol.full_name

    def test_recreate_existing_volume(self):
        vol = self.create_volume('volume01')

        vol.create()
        info = self.client.inspect_volume(vol.full_name)
        assert info['Name'] == vol.full_name

        vol.create()
        info = self.client.inspect_volume(vol.full_name)
        assert info['Name'] == vol.full_name

    def test_inspect_volume(self):
        vol = self.create_volume('volume01')
        vol.create()
        info = vol.inspect()
        assert info['Name'] == vol.full_name

    def test_remove_volume(self):
        vol = Volume(self.client, 'composetest', 'volume01')
        vol.create()
        vol.remove()
        volumes = self.client.volumes()['Volumes']
        assert len([v for v in volumes if v['Name'] == vol.full_name]) == 0

    def test_external_volume(self):
        vol = self.create_volume('composetest_volume_ext', external=True)
        assert vol.external is True
        assert vol.full_name == vol.name
        vol.create()
        info = vol.inspect()
        assert info['Name'] == vol.name

    def test_external_aliased_volume(self):
        alias_name = 'composetest_alias01'
        vol = self.create_volume('volume01', external=alias_name)
        assert vol.external is True
        assert vol.full_name == alias_name
        vol.create()
        info = vol.inspect()
        assert info['Name'] == alias_name

    def test_exists(self):
        vol = self.create_volume('volume01')
        assert vol.exists() is False
        vol.create()
        assert vol.exists() is True

    def test_exists_external(self):
        vol = self.create_volume('volume01', external=True)
        assert vol.exists() is False
        vol.create()
        assert vol.exists() is True

    def test_exists_external_aliased(self):
        vol = self.create_volume('volume01', external='composetest_alias01')
        assert vol.exists() is False
        vol.create()
        assert vol.exists() is True

    def test_volume_default_labels(self):
        vol = self.create_volume('volume01')
        vol.create()
        vol_data = vol.inspect()
        labels = vol_data['Labels']
        assert labels[LABEL_VOLUME] == vol.name
        assert labels[LABEL_PROJECT] == vol.project
