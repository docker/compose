from __future__ import unicode_literals
from __future__ import absolute_import
import os

from .. import unittest
import mock

import docker
from requests import Response

from fig import Service
from fig.container import Container
from fig.service import (
    ConfigError,
    split_port,
    parse_volume_spec,
    build_volume_binding,
    APIError,
)


class ServiceTest(unittest.TestCase):

    def setUp(self):
        self.mock_client = mock.create_autospec(docker.Client)

    def test_name_validations(self):
        self.assertRaises(ConfigError, lambda: Service(name=''))

        self.assertRaises(ConfigError, lambda: Service(name=' '))
        self.assertRaises(ConfigError, lambda: Service(name='/'))
        self.assertRaises(ConfigError, lambda: Service(name='!'))
        self.assertRaises(ConfigError, lambda: Service(name='\xe2'))
        self.assertRaises(ConfigError, lambda: Service(name='_'))
        self.assertRaises(ConfigError, lambda: Service(name='____'))
        self.assertRaises(ConfigError, lambda: Service(name='foo_bar'))
        self.assertRaises(ConfigError, lambda: Service(name='__foo_bar__'))

        Service('a')
        Service('foo')

    def test_project_validation(self):
        self.assertRaises(ConfigError, lambda: Service(name='foo', project='_'))
        Service(name='foo', project='bar')

    def test_config_validation(self):
        self.assertRaises(ConfigError, lambda: Service(name='foo', port=['8000']))
        Service(name='foo', ports=['8000'])

    def test_get_volumes_from_container(self):
        container_id = 'aabbccddee'
        service = Service(
            'test',
            volumes_from=[mock.Mock(id=container_id, spec=Container)])

        self.assertEqual(service._get_volumes_from(), [container_id])

    def test_get_volumes_from_intermediate_container(self):
        container_id = 'aabbccddee'
        service = Service('test')
        container = mock.Mock(id=container_id, spec=Container)

        self.assertEqual(service._get_volumes_from(container), [container_id])

    def test_get_volumes_from_service_container_exists(self):
        container_ids = ['aabbccddee', '12345']
        from_service = mock.create_autospec(Service)
        from_service.containers.return_value = [
            mock.Mock(id=container_id, spec=Container)
            for container_id in container_ids
        ]
        service = Service('test', volumes_from=[from_service])

        self.assertEqual(service._get_volumes_from(), container_ids)

    def test_get_volumes_from_service_no_container(self):
        container_id = 'abababab'
        from_service = mock.create_autospec(Service)
        from_service.containers.return_value = []
        from_service.create_container.return_value = mock.Mock(
            id=container_id,
            spec=Container)
        service = Service('test', volumes_from=[from_service])

        self.assertEqual(service._get_volumes_from(), [container_id])
        from_service.create_container.assert_called_once_with()

    def test_split_port_with_host_ip(self):
        internal_port, external_port = split_port("127.0.0.1:1000:2000")
        self.assertEqual(internal_port, "2000")
        self.assertEqual(external_port, ("127.0.0.1", "1000"))

    def test_split_port_with_protocol(self):
        internal_port, external_port = split_port("127.0.0.1:1000:2000/udp")
        self.assertEqual(internal_port, "2000/udp")
        self.assertEqual(external_port, ("127.0.0.1", "1000"))

    def test_split_port_with_host_ip_no_port(self):
        internal_port, external_port = split_port("127.0.0.1::2000")
        self.assertEqual(internal_port, "2000")
        self.assertEqual(external_port, ("127.0.0.1", None))

    def test_split_port_with_host_port(self):
        internal_port, external_port = split_port("1000:2000")
        self.assertEqual(internal_port, "2000")
        self.assertEqual(external_port, "1000")

    def test_split_port_no_host_port(self):
        internal_port, external_port = split_port("2000")
        self.assertEqual(internal_port, "2000")
        self.assertEqual(external_port, None)

    def test_split_port_invalid(self):
        with self.assertRaises(ConfigError):
            split_port("0.0.0.0:1000:2000:tcp")

    def test_split_domainname_none(self):
        service = Service('foo', hostname='name', client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({})
        self.assertEqual(opts['hostname'], 'name', 'hostname')
        self.assertFalse('domainname' in opts, 'domainname')

    def test_split_domainname_fqdn(self):
        service = Service('foo',
                hostname='name.domain.tld',
                client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({})
        self.assertEqual(opts['hostname'], 'name', 'hostname')
        self.assertEqual(opts['domainname'], 'domain.tld', 'domainname')

    def test_split_domainname_both(self):
        service = Service('foo',
                hostname='name',
                domainname='domain.tld',
                client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({})
        self.assertEqual(opts['hostname'], 'name', 'hostname')
        self.assertEqual(opts['domainname'], 'domain.tld', 'domainname')

    def test_split_domainname_weird(self):
        service = Service('foo',
                hostname='name.sub',
                domainname='domain.tld',
                client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({})
        self.assertEqual(opts['hostname'], 'name.sub', 'hostname')
        self.assertEqual(opts['domainname'], 'domain.tld', 'domainname')

    def test_get_container_not_found(self):
        mock_client = mock.create_autospec(docker.Client)
        mock_client.containers.return_value = []
        service = Service('foo', client=mock_client)

        self.assertRaises(ValueError, service.get_container)

    @mock.patch('fig.service.Container', autospec=True)
    def test_get_container(self, mock_container_class):
        mock_client = mock.create_autospec(docker.Client)
        container_dict = dict(Name='default_foo_2')
        mock_client.containers.return_value = [container_dict]
        service = Service('foo', client=mock_client)

        container = service.get_container(number=2)
        self.assertEqual(container, mock_container_class.from_ps.return_value)
        mock_container_class.from_ps.assert_called_once_with(
            mock_client, container_dict)

    @mock.patch('fig.service.log', autospec=True)
    def test_pull_image(self, mock_log):
        service = Service('foo', client=self.mock_client, image='someimage:sometag')
        service.pull(insecure_registry=True)
        self.mock_client.pull.assert_called_once_with('someimage:sometag', insecure_registry=True)
        mock_log.info.assert_called_once_with('Pulling foo (someimage:sometag)...')

    @mock.patch('fig.service.log', autospec=True)
    def test_create_container_from_insecure_registry(self, mock_log):
        service = Service('foo', client=self.mock_client, image='someimage:sometag')
        mock_response = mock.Mock(Response)
        mock_response.status_code = 404
        mock_response.reason = "Not Found"
        Container.create = mock.Mock()
        Container.create.side_effect = APIError('Mock error', mock_response, "No such image")
        try:
            service.create_container(insecure_registry=True)
        except APIError:  # We expect the APIError because our service requires a non-existent image.
            pass
        self.mock_client.pull.assert_called_once_with('someimage:sometag', insecure_registry=True, stream=True)
        mock_log.info.assert_called_once_with('Pulling image someimage:sometag...')


class ServiceVolumesTest(unittest.TestCase):

    def test_parse_volume_spec_only_one_path(self):
        spec = parse_volume_spec('/the/volume')
        self.assertEqual(spec, (None, '/the/volume', 'rw'))

    def test_parse_volume_spec_internal_and_external(self):
        spec = parse_volume_spec('external:interval')
        self.assertEqual(spec, ('external', 'interval', 'rw'))

    def test_parse_volume_spec_with_mode(self):
        spec = parse_volume_spec('external:interval:ro')
        self.assertEqual(spec, ('external', 'interval', 'ro'))

    def test_parse_volume_spec_too_many_parts(self):
        with self.assertRaises(ConfigError):
            parse_volume_spec('one:two:three:four')

    def test_parse_volume_bad_mode(self):
        with self.assertRaises(ConfigError):
            parse_volume_spec('one:two:notrw')

    def test_build_volume_binding(self):
        binding = build_volume_binding(parse_volume_spec('/outside:/inside'))
        self.assertEqual(
            binding,
            ('/outside', dict(bind='/inside', ro=False)))

    @mock.patch.dict(os.environ)
    def test_build_volume_binding_with_environ(self):
        os.environ['VOLUME_PATH'] = '/opt'
        binding = build_volume_binding(parse_volume_spec('${VOLUME_PATH}:/opt'))
        self.assertEqual(binding, ('/opt', dict(bind='/opt', ro=False)))

    @mock.patch.dict(os.environ)
    def test_building_volume_binding_with_home(self):
        os.environ['HOME'] = '/home/user'
        binding = build_volume_binding(parse_volume_spec('~:/home/user'))
        self.assertEqual(
            binding,
            ('/home/user', dict(bind='/home/user', ro=False)))
