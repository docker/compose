from __future__ import unicode_literals
from __future__ import absolute_import

from .. import unittest
import mock

import docker

from compose.service import Service
from compose.container import Container
from compose.const import LABEL_SERVICE, LABEL_PROJECT, LABEL_ONE_OFF
from compose.service import (
    ConfigError,
    NeedsBuildError,
    build_port_bindings,
    build_volume_binding,
    get_container_data_volumes,
    merge_volume_bindings,
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

        Service('a', image='foo')
        Service('foo', image='foo')

    def test_project_validation(self):
        self.assertRaises(ConfigError, lambda: Service('bar'))
        self.assertRaises(ConfigError, lambda: Service(name='foo', project='_', image='foo'))
        Service(name='foo', project='bar', image='foo')

    def test_containers(self):
        service = Service('db', self.mock_client, 'myproject', image='foo')
        self.mock_client.containers.return_value = []
        self.assertEqual(service.containers(), [])

    def test_containers_with_containers(self):
        self.mock_client.containers.return_value = [
            dict(Name=str(i), Image='foo', Id=i) for i in range(3)
        ]
        service = Service('db', self.mock_client, 'myproject', image='foo')
        self.assertEqual([c.id for c in service.containers()], range(3))

        expected_labels = [
            '{0}=myproject'.format(LABEL_PROJECT),
            '{0}=db'.format(LABEL_SERVICE),
            '{0}=False'.format(LABEL_ONE_OFF),
        ]

        self.mock_client.containers.assert_called_once_with(
            all=False,
            filters={'label': expected_labels})

    def test_get_volumes_from_container(self):
        container_id = 'aabbccddee'
        service = Service(
            'test',
            image='foo',
            volumes_from=[mock.Mock(id=container_id, spec=Container)])

        self.assertEqual(service._get_volumes_from(), [container_id])

    def test_get_volumes_from_service_container_exists(self):
        container_ids = ['aabbccddee', '12345']
        from_service = mock.create_autospec(Service)
        from_service.containers.return_value = [
            mock.Mock(id=container_id, spec=Container)
            for container_id in container_ids
        ]
        service = Service('test', volumes_from=[from_service], image='foo')

        self.assertEqual(service._get_volumes_from(), container_ids)

    def test_get_volumes_from_service_no_container(self):
        container_id = 'abababab'
        from_service = mock.create_autospec(Service)
        from_service.containers.return_value = []
        from_service.create_container.return_value = mock.Mock(
            id=container_id,
            spec=Container)
        service = Service('test', image='foo', volumes_from=[from_service])

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
        service = Service('foo', image='foo', hostname='name', client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        self.assertEqual(opts['hostname'], 'name', 'hostname')
        self.assertFalse('domainname' in opts, 'domainname')

    def test_split_domainname_fqdn(self):
        service = Service(
            'foo',
            hostname='name.domain.tld',
            image='foo',
            client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        self.assertEqual(opts['hostname'], 'name', 'hostname')
        self.assertEqual(opts['domainname'], 'domain.tld', 'domainname')

    def test_split_domainname_both(self):
        service = Service(
            'foo',
            hostname='name',
            image='foo',
            domainname='domain.tld',
            client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        self.assertEqual(opts['hostname'], 'name', 'hostname')
        self.assertEqual(opts['domainname'], 'domain.tld', 'domainname')

    def test_split_domainname_weird(self):
        service = Service(
            'foo',
            hostname='name.sub',
            domainname='domain.tld',
            image='foo',
            client=self.mock_client)
        self.mock_client.containers.return_value = []
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        self.assertEqual(opts['hostname'], 'name.sub', 'hostname')
        self.assertEqual(opts['domainname'], 'domain.tld', 'domainname')

    def test_get_container_not_found(self):
        self.mock_client.containers.return_value = []
        service = Service('foo', client=self.mock_client, image='foo')

        self.assertRaises(ValueError, service.get_container)

    @mock.patch('compose.service.Container', autospec=True)
    def test_get_container(self, mock_container_class):
        container_dict = dict(Name='default_foo_2')
        self.mock_client.containers.return_value = [container_dict]
        service = Service('foo', image='foo', client=self.mock_client)

        container = service.get_container(number=2)
        self.assertEqual(container, mock_container_class.from_ps.return_value)
        mock_container_class.from_ps.assert_called_once_with(
            self.mock_client, container_dict)

    @mock.patch('compose.service.log', autospec=True)
    def test_pull_image(self, mock_log):
        service = Service('foo', client=self.mock_client, image='someimage:sometag')
        service.pull(insecure_registry=True)
        self.mock_client.pull.assert_called_once_with(
            'someimage',
            tag='sometag',
            insecure_registry=True,
            stream=True)
        mock_log.info.assert_called_once_with('Pulling foo (someimage:sometag)...')

    def test_pull_image_no_tag(self):
        service = Service('foo', client=self.mock_client, image='ababab')
        service.pull()
        self.mock_client.pull.assert_called_once_with(
            'ababab',
            tag='latest',
            insecure_registry=False,
            stream=True)

    def test_create_container_from_insecure_registry(self):
        service = Service('foo', client=self.mock_client, image='someimage:sometag')
        images = []

        def pull(repo, tag=None, insecure_registry=False, **kwargs):
            self.assertEqual('someimage', repo)
            self.assertEqual('sometag', tag)
            self.assertTrue(insecure_registry)
            images.append({'Id': 'abc123'})
            return []

        service.image = lambda: images[0] if images else None
        self.mock_client.pull = pull

        service.create_container(insecure_registry=True)
        self.assertEqual(1, len(images))

    @mock.patch('compose.service.Container', autospec=True)
    def test_recreate_container(self, _):
        mock_container = mock.create_autospec(Container)
        service = Service('foo', client=self.mock_client, image='someimage')
        service.image = lambda: {'Id': 'abc123'}
        new_container = service.recreate_container(mock_container)

        mock_container.stop.assert_called_once_with()
        self.mock_client.rename.assert_called_once_with(
            mock_container.id,
            '%s_%s' % (mock_container.short_id, mock_container.name))

        new_container.start.assert_called_once_with()
        mock_container.remove.assert_called_once_with()

    def test_parse_repository_tag(self):
        self.assertEqual(parse_repository_tag("root"), ("root", ""))
        self.assertEqual(parse_repository_tag("root:tag"), ("root", "tag"))
        self.assertEqual(parse_repository_tag("user/repo"), ("user/repo", ""))
        self.assertEqual(parse_repository_tag("user/repo:tag"), ("user/repo", "tag"))
        self.assertEqual(parse_repository_tag("url:5000/repo"), ("url:5000/repo", ""))
        self.assertEqual(parse_repository_tag("url:5000/repo:tag"), ("url:5000/repo", "tag"))

    @mock.patch('compose.service.Container', autospec=True)
    def test_create_container_latest_is_used_when_no_tag_specified(self, mock_container):
        service = Service('foo', client=self.mock_client, image='someimage')
        images = []

        def pull(repo, tag=None, **kwargs):
            self.assertEqual('someimage', repo)
            self.assertEqual('latest', tag)
            images.append({'Id': 'abc123'})
            return []

        service.image = lambda: images[0] if images else None
        self.mock_client.pull = pull

        service.create_container()
        self.assertEqual(1, len(images))

    def test_create_container_with_build(self):
        service = Service('foo', client=self.mock_client, build='.')

        images = []
        service.image = lambda *args, **kwargs: images[0] if images else None
        service.build = lambda: images.append({'Id': 'abc123'})

        service.create_container(do_build=True)
        self.assertEqual(1, len(images))

    def test_create_container_no_build(self):
        service = Service('foo', client=self.mock_client, build='.')
        service.image = lambda: {'Id': 'abc123'}

        service.create_container(do_build=False)
        self.assertFalse(self.mock_client.build.called)

    def test_create_container_no_build_but_needs_build(self):
        service = Service('foo', client=self.mock_client, build='.')
        service.image = lambda: None

        with self.assertRaises(NeedsBuildError):
            service.create_container(do_build=False)


class ServiceVolumesTest(unittest.TestCase):

    def setUp(self):
        self.mock_client = mock.create_autospec(docker.Client)

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
        self.assertEqual(binding, ('/inside', '/outside:/inside:rw'))

    def test_get_container_data_volumes(self):
        options = [
            '/host/volume:/host/volume:ro',
            '/new/volume',
            '/existing/volume',
        ]

        self.mock_client.inspect_image.return_value = {
            'ContainerConfig': {
                'Volumes': {
                    '/mnt/image/data': {},
                }
            }
        }
        container = Container(self.mock_client, {
            'Image': 'ababab',
            'Volumes': {
                '/host/volume': '/host/volume',
                '/existing/volume': '/var/lib/docker/aaaaaaaa',
                '/removed/volume': '/var/lib/docker/bbbbbbbb',
                '/mnt/image/data': '/var/lib/docker/cccccccc',
            },
        }, has_been_inspected=True)

        expected = {
            '/existing/volume': '/var/lib/docker/aaaaaaaa:/existing/volume:rw',
            '/mnt/image/data': '/var/lib/docker/cccccccc:/mnt/image/data:rw',
        }

        binds = get_container_data_volumes(container, options)
        self.assertEqual(binds, expected)

    def test_merge_volume_bindings(self):
        options = [
            '/host/volume:/host/volume:ro',
            '/host/rw/volume:/host/rw/volume',
            '/new/volume',
            '/existing/volume',
        ]

        self.mock_client.inspect_image.return_value = {
            'ContainerConfig': {'Volumes': {}}
        }

        intermediate_container = Container(self.mock_client, {
            'Image': 'ababab',
            'Volumes': {'/existing/volume': '/var/lib/docker/aaaaaaaa'},
        }, has_been_inspected=True)

        expected = [
            '/host/volume:/host/volume:ro',
            '/host/rw/volume:/host/rw/volume:rw',
            '/var/lib/docker/aaaaaaaa:/existing/volume:rw',
        ]

        binds = merge_volume_bindings(options, intermediate_container)
        self.assertEqual(set(binds), set(expected))

    def test_mount_same_host_path_to_two_volumes(self):
        service = Service(
            'web',
            image='busybox',
            volumes=[
                '/host/path:/data1',
                '/host/path:/data2',
            ],
            client=self.mock_client,
        )

        self.mock_client.inspect_image.return_value = {
            'Id': 'ababab',
            'ContainerConfig': {
                'Volumes': {}
            }
        }

        create_options = service._get_container_create_options(
            override_options={},
            number=1,
        )

        self.assertEqual(
            set(create_options['host_config']['Binds']),
            set([
                '/host/path:/data1:rw',
                '/host/path:/data2:rw',
            ]),
        )

    def test_different_host_path_in_container_json(self):
        service = Service(
            'web',
            image='busybox',
            volumes=['/host/path:/data'],
            client=self.mock_client,
        )

        self.mock_client.inspect_image.return_value = {
            'Id': 'ababab',
            'ContainerConfig': {
                'Volumes': {
                    '/data': {},
                }
            }
        }

        self.mock_client.inspect_container.return_value = {
            'Id': '123123123',
            'Image': 'ababab',
            'Volumes': {
                '/data': '/mnt/sda1/host/path',
            },
        }

        create_options = service._get_container_create_options(
            override_options={},
            number=1,
            previous_container=Container(self.mock_client, {'Id': '123123123'}),
        )

        self.assertEqual(
            create_options['host_config']['Binds'],
            ['/mnt/sda1/host/path:/data:rw'],
        )
