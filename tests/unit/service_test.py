from __future__ import unicode_literals
from __future__ import absolute_import

from .. import unittest
import mock

import docker
from requests import Response

from compose import Service
from compose.container import Container
from compose.service import (
    APIError,
    ConfigError,
    build_port_bindings,
    build_volume_binding,
    get_container_name,
    parse_repository_tag,
    parse_volume_spec,
    split_port,
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

    def test_get_container_name(self):
        self.assertIsNone(get_container_name({}))
        self.assertEqual(get_container_name({'Name': 'myproject_db_1'}), 'myproject_db_1')
        self.assertEqual(get_container_name({'Names': ['/myproject_db_1', '/myproject_web_1/db']}), 'myproject_db_1')
        self.assertEqual(get_container_name({'Names': ['/swarm-host-1/myproject_db_1', '/swarm-host-1/myproject_web_1/db']}), 'myproject_db_1')

    def test_containers(self):
        service = Service('db', client=self.mock_client, project='myproject')

        self.mock_client.containers.return_value = []
        self.assertEqual(service.containers(), [])

        self.mock_client.containers.return_value = [
            {'Image': 'busybox', 'Id': 'OUT_1', 'Names': ['/myproject', '/foo/bar']},
            {'Image': 'busybox', 'Id': 'OUT_2', 'Names': ['/myproject_db']},
            {'Image': 'busybox', 'Id': 'OUT_3', 'Names': ['/db_1']},
            {'Image': 'busybox', 'Id': 'IN_1', 'Names': ['/myproject_db_1', '/myproject_web_1/db']},
        ]
        self.assertEqual([c.id for c in service.containers()], ['IN_1'])

    def test_containers_prefixed(self):
        service = Service('db', client=self.mock_client, project='myproject')

        self.mock_client.containers.return_value = [
            {'Image': 'busybox', 'Id': 'OUT_1', 'Names': ['/swarm-host-1/myproject', '/swarm-host-1/foo/bar']},
            {'Image': 'busybox', 'Id': 'OUT_2', 'Names': ['/swarm-host-1/myproject_db']},
            {'Image': 'busybox', 'Id': 'OUT_3', 'Names': ['/swarm-host-1/db_1']},
            {'Image': 'busybox', 'Id': 'IN_1', 'Names': ['/swarm-host-1/myproject_db_1', '/swarm-host-1/myproject_web_1/db']},
        ]
        self.assertEqual([c.id for c in service.containers()], ['IN_1'])

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

    def test_build_port_bindings_with_one_port(self):
        port_bindings = build_port_bindings(["127.0.0.1:1000:1000"])
        self.assertEqual(port_bindings["1000"], [("127.0.0.1", "1000")])

    def test_build_port_bindings_with_matching_internal_ports(self):
        port_bindings = build_port_bindings(["127.0.0.1:1000:1000", "127.0.0.1:2000:1000"])
        self.assertEqual(port_bindings["1000"], [("127.0.0.1", "1000"), ("127.0.0.1", "2000")])

    def test_build_port_bindings_with_nonmatching_internal_ports(self):
        port_bindings = build_port_bindings(["127.0.0.1:1000:1000", "127.0.0.1:2000:2000"])
        self.assertEqual(port_bindings["1000"], [("127.0.0.1", "1000")])
        self.assertEqual(port_bindings["2000"], [("127.0.0.1", "2000")])

    def test_split_domainname_none(self):
        service = Service('foo', hostname='name', client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({'image': 'foo'})
        self.assertEqual(opts['hostname'], 'name', 'hostname')
        self.assertFalse('domainname' in opts, 'domainname')

    def test_split_domainname_fqdn(self):
        service = Service(
            'foo',
            hostname='name.domain.tld',
            client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({'image': 'foo'})
        self.assertEqual(opts['hostname'], 'name', 'hostname')
        self.assertEqual(opts['domainname'], 'domain.tld', 'domainname')

    def test_split_domainname_both(self):
        service = Service(
            'foo',
            hostname='name',
            domainname='domain.tld',
            client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({'image': 'foo'})
        self.assertEqual(opts['hostname'], 'name', 'hostname')
        self.assertEqual(opts['domainname'], 'domain.tld', 'domainname')

    def test_split_domainname_weird(self):
        service = Service(
            'foo',
            hostname='name.sub',
            domainname='domain.tld',
            client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({'image': 'foo'})
        self.assertEqual(opts['hostname'], 'name.sub', 'hostname')
        self.assertEqual(opts['domainname'], 'domain.tld', 'domainname')

    def test_get_container_not_found(self):
        self.mock_client.containers.return_value = []
        service = Service('foo', client=self.mock_client)

        self.assertRaises(ValueError, service.get_container)

    @mock.patch('compose.service.Container', autospec=True)
    def test_get_container(self, mock_container_class):
        container_dict = dict(Name='default_foo_2')
        self.mock_client.containers.return_value = [container_dict]
        service = Service('foo', client=self.mock_client)

        container = service.get_container(number=2)
        self.assertEqual(container, mock_container_class.from_ps.return_value)
        mock_container_class.from_ps.assert_called_once_with(
            self.mock_client, container_dict)

    @mock.patch('compose.service.log', autospec=True)
    def test_pull_image(self, mock_log):
        service = Service('foo', client=self.mock_client, image='someimage:sometag')
        service.pull(insecure_registry=True)
        self.mock_client.pull.assert_called_once_with('someimage:sometag', insecure_registry=True)
        mock_log.info.assert_called_once_with('Pulling foo (someimage:sometag)...')

    @mock.patch('compose.service.Container', autospec=True)
    @mock.patch('compose.service.log', autospec=True)
    def test_create_container_from_insecure_registry(
            self,
            mock_log,
            mock_container):
        service = Service('foo', client=self.mock_client, image='someimage:sometag')
        mock_response = mock.Mock(Response)
        mock_response.status_code = 404
        mock_response.reason = "Not Found"
        mock_container.create.side_effect = APIError(
            'Mock error', mock_response, "No such image")

        # We expect the APIError because our service requires a
        # non-existent image.
        with self.assertRaises(APIError):
            service.create_container(insecure_registry=True)

        self.mock_client.pull.assert_called_once_with(
            'someimage:sometag',
            insecure_registry=True,
            stream=True)
        mock_log.info.assert_called_once_with(
            'Pulling image someimage:sometag...')

    def test_parse_repository_tag(self):
        self.assertEqual(parse_repository_tag("root"), ("root", ""))
        self.assertEqual(parse_repository_tag("root:tag"), ("root", "tag"))
        self.assertEqual(parse_repository_tag("user/repo"), ("user/repo", ""))
        self.assertEqual(parse_repository_tag("user/repo:tag"), ("user/repo", "tag"))
        self.assertEqual(parse_repository_tag("url:5000/repo"), ("url:5000/repo", ""))
        self.assertEqual(parse_repository_tag("url:5000/repo:tag"), ("url:5000/repo", "tag"))

    def test_latest_is_used_when_tag_is_not_specified(self):
        service = Service('foo', client=self.mock_client, image='someimage')
        Container.create = mock.Mock()
        service.create_container()
        self.assertEqual(Container.create.call_args[1]['image'], 'someimage:latest')

    def test_create_container_with_build(self):
        self.mock_client.images.return_value = []
        service = Service('foo', client=self.mock_client, build='.')
        service.build = mock.create_autospec(service.build)
        service.create_container(do_build=True)

        self.mock_client.images.assert_called_once_with(name=service.full_name)
        service.build.assert_called_once_with()

    def test_create_container_no_build(self):
        self.mock_client.images.return_value = []
        service = Service('foo', client=self.mock_client, build='.')
        service.create_container(do_build=False)

        self.assertFalse(self.mock_client.images.called)
        self.assertFalse(self.mock_client.build.called)


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
