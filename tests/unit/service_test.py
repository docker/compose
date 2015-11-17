from __future__ import absolute_import
from __future__ import unicode_literals

import docker

from .. import mock
from .. import unittest
from compose.config.types import VolumeFromSpec
from compose.config.types import VolumeSpec
from compose.const import LABEL_CONFIG_HASH
from compose.const import LABEL_ONE_OFF
from compose.const import LABEL_PROJECT
from compose.const import LABEL_SERVICE
from compose.container import Container
from compose.service import build_ulimits
from compose.service import build_volume_binding
from compose.service import ContainerNet
from compose.service import get_container_data_volumes
from compose.service import merge_volume_bindings
from compose.service import NeedsBuildError
from compose.service import Net
from compose.service import NoSuchImageError
from compose.service import parse_repository_tag
from compose.service import Service
from compose.service import ServiceNet
from compose.service import warn_on_masked_volume


class ServiceTest(unittest.TestCase):

    def setUp(self):
        self.mock_client = mock.create_autospec(docker.Client)

    def test_containers(self):
        service = Service('db', self.mock_client, 'myproject', image='foo')
        self.mock_client.containers.return_value = []
        self.assertEqual(list(service.containers()), [])

    def test_containers_with_containers(self):
        self.mock_client.containers.return_value = [
            dict(Name=str(i), Image='foo', Id=i) for i in range(3)
        ]
        service = Service('db', self.mock_client, 'myproject', image='foo')
        self.assertEqual([c.id for c in service.containers()], list(range(3)))

        expected_labels = [
            '{0}=myproject'.format(LABEL_PROJECT),
            '{0}=db'.format(LABEL_SERVICE),
            '{0}=False'.format(LABEL_ONE_OFF),
        ]

        self.mock_client.containers.assert_called_once_with(
            all=False,
            filters={'label': expected_labels})

    def test_container_without_name(self):
        self.mock_client.containers.return_value = [
            {'Image': 'foo', 'Id': '1', 'Name': '1'},
            {'Image': 'foo', 'Id': '2', 'Name': None},
            {'Image': 'foo', 'Id': '3'},
        ]
        service = Service('db', self.mock_client, 'myproject', image='foo')

        self.assertEqual([c.id for c in service.containers()], ['1'])
        self.assertEqual(service._next_container_number(), 2)
        self.assertEqual(service.get_container(1).id, '1')

    def test_get_volumes_from_container(self):
        container_id = 'aabbccddee'
        service = Service(
            'test',
            image='foo',
            volumes_from=[VolumeFromSpec(mock.Mock(id=container_id, spec=Container), 'rw')])

        self.assertEqual(service._get_volumes_from(), [container_id + ':rw'])

    def test_get_volumes_from_container_read_only(self):
        container_id = 'aabbccddee'
        service = Service(
            'test',
            image='foo',
            volumes_from=[VolumeFromSpec(mock.Mock(id=container_id, spec=Container), 'ro')])

        self.assertEqual(service._get_volumes_from(), [container_id + ':ro'])

    def test_get_volumes_from_service_container_exists(self):
        container_ids = ['aabbccddee', '12345']
        from_service = mock.create_autospec(Service)
        from_service.containers.return_value = [
            mock.Mock(id=container_id, spec=Container)
            for container_id in container_ids
        ]
        service = Service('test', volumes_from=[VolumeFromSpec(from_service, 'rw')], image='foo')

        self.assertEqual(service._get_volumes_from(), [container_ids[0] + ":rw"])

    def test_get_volumes_from_service_container_exists_with_flags(self):
        for mode in ['ro', 'rw', 'z', 'rw,z', 'z,rw']:
            container_ids = ['aabbccddee:' + mode, '12345:' + mode]
            from_service = mock.create_autospec(Service)
            from_service.containers.return_value = [
                mock.Mock(id=container_id.split(':')[0], spec=Container)
                for container_id in container_ids
            ]
            service = Service('test', volumes_from=[VolumeFromSpec(from_service, mode)], image='foo')

            self.assertEqual(service._get_volumes_from(), [container_ids[0]])

    def test_get_volumes_from_service_no_container(self):
        container_id = 'abababab'
        from_service = mock.create_autospec(Service)
        from_service.containers.return_value = []
        from_service.create_container.return_value = mock.Mock(
            id=container_id,
            spec=Container)
        service = Service('test', image='foo', volumes_from=[VolumeFromSpec(from_service, 'rw')])

        self.assertEqual(service._get_volumes_from(), [container_id + ':rw'])
        from_service.create_container.assert_called_once_with()

    def test_split_domainname_none(self):
        service = Service('foo', image='foo', hostname='name', client=self.mock_client)
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        self.assertEqual(opts['hostname'], 'name', 'hostname')
        self.assertFalse('domainname' in opts, 'domainname')

    def test_memory_swap_limit(self):
        self.mock_client.create_host_config.return_value = {}

        service = Service(name='foo', image='foo', hostname='name', client=self.mock_client, mem_limit=1000000000, memswap_limit=2000000000)
        service._get_container_create_options({'some': 'overrides'}, 1)

        self.assertTrue(self.mock_client.create_host_config.called)
        self.assertEqual(
            self.mock_client.create_host_config.call_args[1]['mem_limit'],
            1000000000
        )
        self.assertEqual(
            self.mock_client.create_host_config.call_args[1]['memswap_limit'],
            2000000000
        )

    def test_cgroup_parent(self):
        self.mock_client.create_host_config.return_value = {}

        service = Service(name='foo', image='foo', hostname='name', client=self.mock_client, cgroup_parent='test')
        service._get_container_create_options({'some': 'overrides'}, 1)

        self.assertTrue(self.mock_client.create_host_config.called)
        self.assertEqual(
            self.mock_client.create_host_config.call_args[1]['cgroup_parent'],
            'test'
        )

    def test_log_opt(self):
        self.mock_client.create_host_config.return_value = {}

        log_opt = {'syslog-address': 'tcp://192.168.0.42:123'}
        service = Service(name='foo', image='foo', hostname='name', client=self.mock_client, log_driver='syslog', log_opt=log_opt)
        service._get_container_create_options({'some': 'overrides'}, 1)

        self.assertTrue(self.mock_client.create_host_config.called)
        self.assertEqual(
            self.mock_client.create_host_config.call_args[1]['log_config'],
            {'Type': 'syslog', 'Config': {'syslog-address': 'tcp://192.168.0.42:123'}}
        )

    def test_split_domainname_fqdn(self):
        service = Service(
            'foo',
            hostname='name.domain.tld',
            image='foo',
            client=self.mock_client)
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
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        self.assertEqual(opts['hostname'], 'name.sub', 'hostname')
        self.assertEqual(opts['domainname'], 'domain.tld', 'domainname')

    def test_no_default_hostname_when_not_using_networking(self):
        service = Service(
            'foo',
            image='foo',
            use_networking=False,
            client=self.mock_client,
        )
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        self.assertIsNone(opts.get('hostname'))

    def test_get_container_create_options_with_name_option(self):
        service = Service(
            'foo',
            image='foo',
            client=self.mock_client,
            container_name='foo1')
        name = 'the_new_name'
        opts = service._get_container_create_options(
            {'name': name},
            1,
            one_off=True)
        self.assertEqual(opts['name'], name)

    def test_get_container_create_options_does_not_mutate_options(self):
        labels = {'thing': 'real'}
        environment = {'also': 'real'}
        service = Service(
            'foo',
            image='foo',
            labels=dict(labels),
            client=self.mock_client,
            environment=dict(environment),
        )
        self.mock_client.inspect_image.return_value = {'Id': 'abcd'}
        prev_container = mock.Mock(
            id='ababab',
            image_config={'ContainerConfig': {}})

        opts = service._get_container_create_options(
            {},
            1,
            previous_container=prev_container)

        self.assertEqual(service.options['labels'], labels)
        self.assertEqual(service.options['environment'], environment)

        self.assertEqual(
            opts['labels'][LABEL_CONFIG_HASH],
            '3c85881a8903b9d73a06c41860c8be08acce1494ab4cf8408375966dccd714de')
        self.assertEqual(
            opts['environment'],
            {
                'affinity:container': '=ababab',
                'also': 'real',
            }
        )

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
        service.pull()
        self.mock_client.pull.assert_called_once_with(
            'someimage',
            tag='sometag',
            stream=True)
        mock_log.info.assert_called_once_with('Pulling foo (someimage:sometag)...')

    def test_pull_image_no_tag(self):
        service = Service('foo', client=self.mock_client, image='ababab')
        service.pull()
        self.mock_client.pull.assert_called_once_with(
            'ababab',
            tag='latest',
            stream=True)

    @mock.patch('compose.service.log', autospec=True)
    def test_pull_image_digest(self, mock_log):
        service = Service('foo', client=self.mock_client, image='someimage@sha256:1234')
        service.pull()
        self.mock_client.pull.assert_called_once_with(
            'someimage',
            tag='sha256:1234',
            stream=True)
        mock_log.info.assert_called_once_with('Pulling foo (someimage@sha256:1234)...')

    @mock.patch('compose.service.Container', autospec=True)
    def test_recreate_container(self, _):
        mock_container = mock.create_autospec(Container)
        service = Service('foo', client=self.mock_client, image='someimage')
        service.image = lambda: {'Id': 'abc123'}
        new_container = service.recreate_container(mock_container)

        mock_container.stop.assert_called_once_with(timeout=10)
        mock_container.rename_to_tmp_name.assert_called_once_with()

        new_container.start.assert_called_once_with()
        mock_container.remove.assert_called_once_with()

    @mock.patch('compose.service.Container', autospec=True)
    def test_recreate_container_with_timeout(self, _):
        mock_container = mock.create_autospec(Container)
        self.mock_client.inspect_image.return_value = {'Id': 'abc123'}
        service = Service('foo', client=self.mock_client, image='someimage')
        service.recreate_container(mock_container, timeout=1)

        mock_container.stop.assert_called_once_with(timeout=1)

    def test_parse_repository_tag(self):
        self.assertEqual(parse_repository_tag("root"), ("root", "", ":"))
        self.assertEqual(parse_repository_tag("root:tag"), ("root", "tag", ":"))
        self.assertEqual(parse_repository_tag("user/repo"), ("user/repo", "", ":"))
        self.assertEqual(parse_repository_tag("user/repo:tag"), ("user/repo", "tag", ":"))
        self.assertEqual(parse_repository_tag("url:5000/repo"), ("url:5000/repo", "", ":"))
        self.assertEqual(parse_repository_tag("url:5000/repo:tag"), ("url:5000/repo", "tag", ":"))

        self.assertEqual(parse_repository_tag("root@sha256:digest"), ("root", "sha256:digest", "@"))
        self.assertEqual(parse_repository_tag("user/repo@sha256:digest"), ("user/repo", "sha256:digest", "@"))
        self.assertEqual(parse_repository_tag("url:5000/repo@sha256:digest"), ("url:5000/repo", "sha256:digest", "@"))

    def test_create_container_with_build(self):
        service = Service('foo', client=self.mock_client, build='.')
        self.mock_client.inspect_image.side_effect = [
            NoSuchImageError,
            {'Id': 'abc123'},
        ]
        self.mock_client.build.return_value = [
            '{"stream": "Successfully built abcd"}',
        ]

        service.create_container(do_build=True)
        self.mock_client.build.assert_called_once_with(
            tag='default_foo',
            dockerfile=None,
            stream=True,
            path='.',
            pull=False,
            forcerm=False,
            nocache=False,
            rm=True,
        )

    def test_create_container_no_build(self):
        service = Service('foo', client=self.mock_client, build='.')
        self.mock_client.inspect_image.return_value = {'Id': 'abc123'}

        service.create_container(do_build=False)
        self.assertFalse(self.mock_client.build.called)

    def test_create_container_no_build_but_needs_build(self):
        service = Service('foo', client=self.mock_client, build='.')
        self.mock_client.inspect_image.side_effect = NoSuchImageError
        with self.assertRaises(NeedsBuildError):
            service.create_container(do_build=False)

    def test_build_does_not_pull(self):
        self.mock_client.build.return_value = [
            b'{"stream": "Successfully built 12345"}',
        ]

        service = Service('foo', client=self.mock_client, build='.')
        service.build()

        self.assertEqual(self.mock_client.build.call_count, 1)
        self.assertFalse(self.mock_client.build.call_args[1]['pull'])

    def test_config_dict(self):
        self.mock_client.inspect_image.return_value = {'Id': 'abcd'}
        service = Service(
            'foo',
            image='example.com/foo',
            client=self.mock_client,
            net=ServiceNet(Service('other')),
            links=[(Service('one'), 'one')],
            volumes_from=[VolumeFromSpec(Service('two'), 'rw')])

        config_dict = service.config_dict()
        expected = {
            'image_id': 'abcd',
            'options': {'image': 'example.com/foo'},
            'links': [('one', 'one')],
            'net': 'other',
            'volumes_from': [('two', 'rw')],
        }
        self.assertEqual(config_dict, expected)

    def test_config_dict_with_net_from_container(self):
        self.mock_client.inspect_image.return_value = {'Id': 'abcd'}
        container = Container(
            self.mock_client,
            {'Id': 'aaabbb', 'Name': '/foo_1'})
        service = Service(
            'foo',
            image='example.com/foo',
            client=self.mock_client,
            net=container)

        config_dict = service.config_dict()
        expected = {
            'image_id': 'abcd',
            'options': {'image': 'example.com/foo'},
            'links': [],
            'net': 'aaabbb',
            'volumes_from': [],
        }
        self.assertEqual(config_dict, expected)

    def test_specifies_host_port_with_no_ports(self):
        service = Service(
            'foo',
            image='foo')
        self.assertEqual(service.specifies_host_port(), False)

    def test_specifies_host_port_with_container_port(self):
        service = Service(
            'foo',
            image='foo',
            ports=["2000"])
        self.assertEqual(service.specifies_host_port(), False)

    def test_specifies_host_port_with_host_port(self):
        service = Service(
            'foo',
            image='foo',
            ports=["1000:2000"])
        self.assertEqual(service.specifies_host_port(), True)

    def test_specifies_host_port_with_host_ip_no_port(self):
        service = Service(
            'foo',
            image='foo',
            ports=["127.0.0.1::2000"])
        self.assertEqual(service.specifies_host_port(), False)

    def test_specifies_host_port_with_host_ip_and_port(self):
        service = Service(
            'foo',
            image='foo',
            ports=["127.0.0.1:1000:2000"])
        self.assertEqual(service.specifies_host_port(), True)

    def test_specifies_host_port_with_container_port_range(self):
        service = Service(
            'foo',
            image='foo',
            ports=["2000-3000"])
        self.assertEqual(service.specifies_host_port(), False)

    def test_specifies_host_port_with_host_port_range(self):
        service = Service(
            'foo',
            image='foo',
            ports=["1000-2000:2000-3000"])
        self.assertEqual(service.specifies_host_port(), True)

    def test_specifies_host_port_with_host_ip_no_port_range(self):
        service = Service(
            'foo',
            image='foo',
            ports=["127.0.0.1::2000-3000"])
        self.assertEqual(service.specifies_host_port(), False)

    def test_specifies_host_port_with_host_ip_and_port_range(self):
        service = Service(
            'foo',
            image='foo',
            ports=["127.0.0.1:1000-2000:2000-3000"])
        self.assertEqual(service.specifies_host_port(), True)

    def test_get_links_with_networking(self):
        service = Service(
            'foo',
            image='foo',
            links=[(Service('one'), 'one')],
            use_networking=True)
        self.assertEqual(service._get_links(link_to_self=True), [])


def sort_by_name(dictionary_list):
    return sorted(dictionary_list, key=lambda k: k['name'])


class BuildUlimitsTestCase(unittest.TestCase):

    def test_build_ulimits_with_dict(self):
        ulimits = build_ulimits(
            {
                'nofile': {'soft': 10000, 'hard': 20000},
                'nproc': {'soft': 65535, 'hard': 65535}
            }
        )
        expected = [
            {'name': 'nofile', 'soft': 10000, 'hard': 20000},
            {'name': 'nproc', 'soft': 65535, 'hard': 65535}
        ]
        assert sort_by_name(ulimits) == sort_by_name(expected)

    def test_build_ulimits_with_ints(self):
        ulimits = build_ulimits({'nofile': 20000, 'nproc': 65535})
        expected = [
            {'name': 'nofile', 'soft': 20000, 'hard': 20000},
            {'name': 'nproc', 'soft': 65535, 'hard': 65535}
        ]
        assert sort_by_name(ulimits) == sort_by_name(expected)

    def test_build_ulimits_with_integers_and_dicts(self):
        ulimits = build_ulimits(
            {
                'nproc': 65535,
                'nofile': {'soft': 10000, 'hard': 20000}
            }
        )
        expected = [
            {'name': 'nofile', 'soft': 10000, 'hard': 20000},
            {'name': 'nproc', 'soft': 65535, 'hard': 65535}
        ]
        assert sort_by_name(ulimits) == sort_by_name(expected)


class NetTestCase(unittest.TestCase):

    def test_net(self):
        net = Net('host')
        self.assertEqual(net.id, 'host')
        self.assertEqual(net.mode, 'host')
        self.assertEqual(net.service_name, None)

    def test_net_container(self):
        container_id = 'abcd'
        net = ContainerNet(Container(None, {'Id': container_id}))
        self.assertEqual(net.id, container_id)
        self.assertEqual(net.mode, 'container:' + container_id)
        self.assertEqual(net.service_name, None)

    def test_net_service(self):
        container_id = 'bbbb'
        service_name = 'web'
        mock_client = mock.create_autospec(docker.Client)
        mock_client.containers.return_value = [
            {'Id': container_id, 'Name': container_id, 'Image': 'abcd'},
        ]

        service = Service(name=service_name, client=mock_client)
        net = ServiceNet(service)

        self.assertEqual(net.id, service_name)
        self.assertEqual(net.mode, 'container:' + container_id)
        self.assertEqual(net.service_name, service_name)

    def test_net_service_no_containers(self):
        service_name = 'web'
        mock_client = mock.create_autospec(docker.Client)
        mock_client.containers.return_value = []

        service = Service(name=service_name, client=mock_client)
        net = ServiceNet(service)

        self.assertEqual(net.id, service_name)
        self.assertEqual(net.mode, None)
        self.assertEqual(net.service_name, service_name)


class ServiceVolumesTest(unittest.TestCase):

    def setUp(self):
        self.mock_client = mock.create_autospec(docker.Client)

    def test_build_volume_binding(self):
        binding = build_volume_binding(VolumeSpec.parse('/outside:/inside'))
        assert binding == ('/inside', '/outside:/inside:rw')

    def test_get_container_data_volumes(self):
        options = [VolumeSpec.parse(v) for v in [
            '/host/volume:/host/volume:ro',
            '/new/volume',
            '/existing/volume',
        ]]

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

        expected = [
            VolumeSpec.parse('/var/lib/docker/aaaaaaaa:/existing/volume:rw'),
            VolumeSpec.parse('/var/lib/docker/cccccccc:/mnt/image/data:rw'),
        ]

        volumes = get_container_data_volumes(container, options)
        assert sorted(volumes) == sorted(expected)

    def test_merge_volume_bindings(self):
        options = [
            VolumeSpec.parse('/host/volume:/host/volume:ro'),
            VolumeSpec.parse('/host/rw/volume:/host/rw/volume'),
            VolumeSpec.parse('/new/volume'),
            VolumeSpec.parse('/existing/volume'),
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
                VolumeSpec.parse('/host/path:/data1'),
                VolumeSpec.parse('/host/path:/data2'),
            ],
            client=self.mock_client,
        )

        self.mock_client.inspect_image.return_value = {
            'Id': 'ababab',
            'ContainerConfig': {
                'Volumes': {}
            }
        }

        service._get_container_create_options(
            override_options={},
            number=1,
        )

        self.assertEqual(
            set(self.mock_client.create_host_config.call_args[1]['binds']),
            set([
                '/host/path:/data1:rw',
                '/host/path:/data2:rw',
            ]),
        )

    def test_different_host_path_in_container_json(self):
        service = Service(
            'web',
            image='busybox',
            volumes=[VolumeSpec.parse('/host/path:/data')],
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

        service._get_container_create_options(
            override_options={},
            number=1,
            previous_container=Container(self.mock_client, {'Id': '123123123'}),
        )

        self.assertEqual(
            self.mock_client.create_host_config.call_args[1]['binds'],
            ['/mnt/sda1/host/path:/data:rw'],
        )

    def test_warn_on_masked_volume_no_warning_when_no_container_volumes(self):
        volumes_option = [VolumeSpec('/home/user', '/path', 'rw')]
        container_volumes = []
        service = 'service_name'

        with mock.patch('compose.service.log', autospec=True) as mock_log:
            warn_on_masked_volume(volumes_option, container_volumes, service)

        assert not mock_log.warn.called

    def test_warn_on_masked_volume_when_masked(self):
        volumes_option = [VolumeSpec('/home/user', '/path', 'rw')]
        container_volumes = [
            VolumeSpec('/var/lib/docker/path', '/path', 'rw'),
            VolumeSpec('/var/lib/docker/path', '/other', 'rw'),
        ]
        service = 'service_name'

        with mock.patch('compose.service.log', autospec=True) as mock_log:
            warn_on_masked_volume(volumes_option, container_volumes, service)

        mock_log.warn.assert_called_once_with(mock.ANY)

    def test_warn_on_masked_no_warning_with_same_path(self):
        volumes_option = [VolumeSpec('/home/user', '/path', 'rw')]
        container_volumes = [VolumeSpec('/home/user', '/path', 'rw')]
        service = 'service_name'

        with mock.patch('compose.service.log', autospec=True) as mock_log:
            warn_on_masked_volume(volumes_option, container_volumes, service)

        assert not mock_log.warn.called

    def test_create_with_special_volume_mode(self):
        self.mock_client.inspect_image.return_value = {'Id': 'imageid'}

        self.mock_client.create_container.return_value = {'Id': 'containerid'}

        volume = '/tmp:/foo:z'
        Service(
            'web',
            client=self.mock_client,
            image='busybox',
            volumes=[VolumeSpec.parse(volume)],
        ).create_container()

        assert self.mock_client.create_container.call_count == 1
        self.assertEqual(
            self.mock_client.create_host_config.call_args[1]['binds'],
            [volume])
