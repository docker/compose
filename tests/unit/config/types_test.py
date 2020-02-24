import pytest

from compose.config.errors import ConfigurationError
from compose.config.types import parse_extra_hosts
from compose.config.types import ServicePort
from compose.config.types import VolumeFromSpec
from compose.config.types import VolumeSpec
from compose.const import COMPOSE_SPEC as VERSION
from compose.const import COMPOSEFILE_V1 as V1


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


class TestServicePort:
    def test_parse_dict(self):
        data = {
            'target': 8000,
            'published': 8000,
            'protocol': 'udp',
            'mode': 'global',
        }
        ports = ServicePort.parse(data)
        assert len(ports) == 1
        assert ports[0].repr() == data

    def test_parse_simple_target_port(self):
        ports = ServicePort.parse(8000)
        assert len(ports) == 1
        assert ports[0].target == 8000

    def test_parse_complete_port_definition(self):
        port_def = '1.1.1.1:3000:3000/udp'
        ports = ServicePort.parse(port_def)
        assert len(ports) == 1
        assert ports[0].repr() == {
            'target': 3000,
            'published': 3000,
            'external_ip': '1.1.1.1',
            'protocol': 'udp',
        }
        assert ports[0].legacy_repr() == port_def

    def test_parse_ext_ip_no_published_port(self):
        port_def = '1.1.1.1::3000'
        ports = ServicePort.parse(port_def)
        assert len(ports) == 1
        assert ports[0].legacy_repr() == port_def + '/tcp'
        assert ports[0].repr() == {
            'target': 3000,
            'external_ip': '1.1.1.1',
        }

    def test_repr_published_port_0(self):
        port_def = '0:4000'
        ports = ServicePort.parse(port_def)
        assert len(ports) == 1
        assert ports[0].legacy_repr() == port_def + '/tcp'

    def test_parse_port_range(self):
        ports = ServicePort.parse('25000-25001:4000-4001')
        assert len(ports) == 2
        reprs = [p.repr() for p in ports]
        assert {
            'target': 4000,
            'published': 25000
        } in reprs
        assert {
            'target': 4001,
            'published': 25001
        } in reprs

    def test_parse_port_publish_range(self):
        ports = ServicePort.parse('4440-4450:4000')
        assert len(ports) == 1
        reprs = [p.repr() for p in ports]
        assert {
            'target': 4000,
            'published': '4440-4450'
        } in reprs

    def test_parse_invalid_port(self):
        port_def = '4000p'
        with pytest.raises(ConfigurationError):
            ServicePort.parse(port_def)

    def test_parse_invalid_publish_range(self):
        port_def = '-4000:4000'
        with pytest.raises(ConfigurationError):
            ServicePort.parse(port_def)

        port_def = 'asdf:4000'
        with pytest.raises(ConfigurationError):
            ServicePort.parse(port_def)

        port_def = '1234-12f:4000'
        with pytest.raises(ConfigurationError):
            ServicePort.parse(port_def)

        port_def = '1234-1235-1239:4000'
        with pytest.raises(ConfigurationError):
            ServicePort.parse(port_def)


class TestVolumeSpec:

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

    def test_parse_volume_windows_absolute_path_normalized(self):
        windows_path = "c:\\Users\\me\\Documents\\shiny\\config:/opt/shiny/config:ro"
        assert VolumeSpec._parse_win32(windows_path, True) == (
            "/c/Users/me/Documents/shiny/config",
            "/opt/shiny/config",
            "ro"
        )

    def test_parse_volume_windows_absolute_path_native(self):
        windows_path = "c:\\Users\\me\\Documents\\shiny\\config:/opt/shiny/config:ro"
        assert VolumeSpec._parse_win32(windows_path, False) == (
            "c:\\Users\\me\\Documents\\shiny\\config",
            "/opt/shiny/config",
            "ro"
        )

    def test_parse_volume_windows_internal_path_normalized(self):
        windows_path = 'C:\\Users\\reimu\\scarlet:C:\\scarlet\\app:ro'
        assert VolumeSpec._parse_win32(windows_path, True) == (
            '/c/Users/reimu/scarlet',
            'C:\\scarlet\\app',
            'ro'
        )

    def test_parse_volume_windows_internal_path_native(self):
        windows_path = 'C:\\Users\\reimu\\scarlet:C:\\scarlet\\app:ro'
        assert VolumeSpec._parse_win32(windows_path, False) == (
            'C:\\Users\\reimu\\scarlet',
            'C:\\scarlet\\app',
            'ro'
        )

    def test_parse_volume_windows_just_drives_normalized(self):
        windows_path = 'E:\\:C:\\:ro'
        assert VolumeSpec._parse_win32(windows_path, True) == (
            '/e/',
            'C:\\',
            'ro'
        )

    def test_parse_volume_windows_just_drives_native(self):
        windows_path = 'E:\\:C:\\:ro'
        assert VolumeSpec._parse_win32(windows_path, False) == (
            'E:\\',
            'C:\\',
            'ro'
        )

    def test_parse_volume_windows_mixed_notations_normalized(self):
        windows_path = 'C:\\Foo:/root/foo'
        assert VolumeSpec._parse_win32(windows_path, True) == (
            '/c/Foo',
            '/root/foo',
            'rw'
        )

    def test_parse_volume_windows_mixed_notations_native(self):
        windows_path = 'C:\\Foo:/root/foo'
        assert VolumeSpec._parse_win32(windows_path, False) == (
            'C:\\Foo',
            '/root/foo',
            'rw'
        )


class TestVolumesFromSpec:

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
        volume_from = VolumeFromSpec.parse('servicea', self.services, VERSION)
        assert volume_from == VolumeFromSpec('servicea', 'rw', 'service')

    def test_parse_v2_from_service_with_mode(self):
        volume_from = VolumeFromSpec.parse('servicea:ro', self.services, VERSION)
        assert volume_from == VolumeFromSpec('servicea', 'ro', 'service')

    def test_parse_v2_from_container(self):
        volume_from = VolumeFromSpec.parse('container:foo', self.services, VERSION)
        assert volume_from == VolumeFromSpec('foo', 'rw', 'container')

    def test_parse_v2_from_container_with_mode(self):
        volume_from = VolumeFromSpec.parse('container:foo:ro', self.services, VERSION)
        assert volume_from == VolumeFromSpec('foo', 'ro', 'container')

    def test_parse_v2_invalid_type(self):
        with pytest.raises(ConfigurationError) as exc:
            VolumeFromSpec.parse('bogus:foo:ro', self.services, VERSION)
        assert "Unknown volumes_from type 'bogus'" in exc.exconly()

    def test_parse_v2_invalid(self):
        with pytest.raises(ConfigurationError):
            VolumeFromSpec.parse('unknown:format:ro', self.services, VERSION)
