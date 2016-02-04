from __future__ import absolute_import
from __future__ import unicode_literals

import pytest

from compose.config.config import V1
from compose.config.config import V2_0
from compose.config.errors import ConfigurationError
from compose.config.types import parse_extra_hosts
from compose.config.types import VolumeFromSpec
from compose.config.types import VolumeSpec
from compose.const import IS_WINDOWS_PLATFORM


def test_parse_extra_hosts_list():
    expected = {'www.example.com': '192.168.0.17'}
    assert parse_extra_hosts(["www.example.com:192.168.0.17"]) == expected

    expected = {'www.example.com': '192.168.0.17'}
    assert parse_extra_hosts(["www.example.com: 192.168.0.17"]) == expected

    assert parse_extra_hosts([
        "www.example.com: 192.168.0.17",
        "static.example.com:192.168.0.19",
        "api.example.com: 192.168.0.18",
        "v6.example.com: ::1"
    ]) == {
        'www.example.com': '192.168.0.17',
        'static.example.com': '192.168.0.19',
        'api.example.com': '192.168.0.18',
        'v6.example.com': '::1'
    }


def test_parse_extra_hosts_dict():
    assert parse_extra_hosts({
        'www.example.com': '192.168.0.17',
        'api.example.com': '192.168.0.18'
    }) == {
        'www.example.com': '192.168.0.17',
        'api.example.com': '192.168.0.18'
    }


class TestVolumeSpec(object):

    def test_parse_volume_spec_only_one_path(self):
        spec = VolumeSpec.parse('/the/volume')
        assert spec == (None, '/the/volume', 'rw')

    def test_parse_volume_spec_internal_and_external(self):
        spec = VolumeSpec.parse('external:interval')
        assert spec == ('external', 'interval', 'rw')

    def test_parse_volume_spec_with_mode(self):
        spec = VolumeSpec.parse('external:interval:ro')
        assert spec == ('external', 'interval', 'ro')

        spec = VolumeSpec.parse('external:interval:z')
        assert spec == ('external', 'interval', 'z')

    def test_parse_volume_spec_too_many_parts(self):
        with pytest.raises(ConfigurationError) as exc:
            VolumeSpec.parse('one:two:three:four')
        assert 'has incorrect format' in exc.exconly()

    @pytest.mark.xfail((not IS_WINDOWS_PLATFORM), reason='does not have a drive')
    def test_parse_volume_windows_absolute_path(self):
        windows_path = "c:\\Users\\me\\Documents\\shiny\\config:\\opt\\shiny\\config:ro"
        assert VolumeSpec.parse(windows_path) == (
            "/c/Users/me/Documents/shiny/config",
            "/opt/shiny/config",
            "ro"
        )


class TestVolumesFromSpec(object):

    services = ['servicea', 'serviceb']

    def test_parse_v1_from_service(self):
        volume_from = VolumeFromSpec.parse('servicea', self.services, V1)
        assert volume_from == VolumeFromSpec('servicea', 'rw', 'service')

    def test_parse_v1_from_container(self):
        volume_from = VolumeFromSpec.parse('foo:ro', self.services, V1)
        assert volume_from == VolumeFromSpec('foo', 'ro', 'container')

    def test_parse_v1_invalid(self):
        with pytest.raises(ConfigurationError):
            VolumeFromSpec.parse('unknown:format:ro', self.services, V1)

    def test_parse_v2_from_service(self):
        volume_from = VolumeFromSpec.parse('servicea', self.services, V2_0)
        assert volume_from == VolumeFromSpec('servicea', 'rw', 'service')

    def test_parse_v2_from_service_with_mode(self):
        volume_from = VolumeFromSpec.parse('servicea:ro', self.services, V2_0)
        assert volume_from == VolumeFromSpec('servicea', 'ro', 'service')

    def test_parse_v2_from_container(self):
        volume_from = VolumeFromSpec.parse('container:foo', self.services, V2_0)
        assert volume_from == VolumeFromSpec('foo', 'rw', 'container')

    def test_parse_v2_from_container_with_mode(self):
        volume_from = VolumeFromSpec.parse('container:foo:ro', self.services, V2_0)
        assert volume_from == VolumeFromSpec('foo', 'ro', 'container')

    def test_parse_v2_invalid_type(self):
        with pytest.raises(ConfigurationError) as exc:
            VolumeFromSpec.parse('bogus:foo:ro', self.services, V2_0)
        assert "Unknown volumes_from type 'bogus'" in exc.exconly()

    def test_parse_v2_invalid(self):
        with pytest.raises(ConfigurationError):
            VolumeFromSpec.parse('unknown:format:ro', self.services, V2_0)
