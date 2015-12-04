import pytest

from compose.config.errors import ConfigurationError
from compose.config.types import parse_extra_hosts
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
        "api.example.com: 192.168.0.18"
    ]) == {
        'www.example.com': '192.168.0.17',
        'static.example.com': '192.168.0.19',
        'api.example.com': '192.168.0.18'
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
