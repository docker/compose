import docker
import pytest
from docker.constants import DEFAULT_DOCKER_API_VERSION
from docker.errors import APIError
from docker.errors import ImageNotFound
from docker.errors import NotFound

from .. import mock
from .. import unittest
from compose.config.errors import DependencyError
from compose.config.types import MountSpec
from compose.config.types import ServicePort
from compose.config.types import ServiceSecret
from compose.config.types import VolumeFromSpec
from compose.config.types import VolumeSpec
from compose.const import API_VERSIONS
from compose.const import LABEL_CONFIG_HASH
from compose.const import LABEL_ONE_OFF
from compose.const import LABEL_PROJECT
from compose.const import LABEL_SERVICE
from compose.const import SECRETS_PATH
from compose.const import WINDOWS_LONGPATH_PREFIX
from compose.container import Container
from compose.errors import OperationFailedError
from compose.parallel import ParallelStreamWriter
from compose.project import OneOffFilter
from compose.service import build_ulimits
from compose.service import build_volume_binding
from compose.service import BuildAction
from compose.service import ContainerNetworkMode
from compose.service import format_environment
from compose.service import formatted_ports
from compose.service import get_container_data_volumes
from compose.service import ImageType
from compose.service import merge_volume_bindings
from compose.service import NeedsBuildError
from compose.service import NetworkMode
from compose.service import NoSuchImageError
from compose.service import parse_repository_tag
from compose.service import rewrite_build_path
from compose.service import Service
from compose.service import ServiceNetworkMode
from compose.service import warn_on_masked_volume


class ServiceTest(unittest.TestCase):

    def setUp(self):
        self.mock_client = mock.create_autospec(docker.APIClient)
        self.mock_client.api_version = DEFAULT_DOCKER_API_VERSION
        self.mock_client._general_configs = {}

    def test_containers(self):
        service = Service('db', self.mock_client, 'myproject', image='foo')
        self.mock_client.containers.return_value = []
        assert list(service.containers()) == []

    def test_containers_with_containers(self):
        self.mock_client.containers.return_value = [
            dict(Name=str(i), Image='foo', Id=i) for i in range(3)
        ]
        service = Service('db', self.mock_client, 'myproject', image='foo')
        assert [c.id for c in service.containers()] == list(range(3))

        expected_labels = [
            '{}=myproject'.format(LABEL_PROJECT),
            '{}=db'.format(LABEL_SERVICE),
            '{}=False'.format(LABEL_ONE_OFF),
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

        assert [c.id for c in service.containers()] == ['1']
        assert service._next_container_number() == 2
        assert service.get_container(1).id == '1'

    def test_get_volumes_from_container(self):
        container_id = 'aabbccddee'
        service = Service(
            'test',
            image='foo',
            volumes_from=[
                VolumeFromSpec(
                    mock.Mock(id=container_id, spec=Container),
                    'rw',
                    'container')])

        assert service._get_volumes_from() == [container_id + ':rw']

    def test_get_volumes_from_container_read_only(self):
        container_id = 'aabbccddee'
        service = Service(
            'test',
            image='foo',
            volumes_from=[
                VolumeFromSpec(
                    mock.Mock(id=container_id, spec=Container),
                    'ro',
                    'container')])

        assert service._get_volumes_from() == [container_id + ':ro']

    def test_get_volumes_from_service_container_exists(self):
        container_ids = ['aabbccddee', '12345']
        from_service = mock.create_autospec(Service)
        from_service.containers.return_value = [
            mock.Mock(id=container_id, spec=Container)
            for container_id in container_ids
        ]
        service = Service(
            'test',
            volumes_from=[VolumeFromSpec(from_service, 'rw', 'service')],
            image='foo')

        assert service._get_volumes_from() == [container_ids[0] + ":rw"]

    def test_get_volumes_from_service_container_exists_with_flags(self):
        for mode in ['ro', 'rw', 'z', 'rw,z', 'z,rw']:
            container_ids = ['aabbccddee:' + mode, '12345:' + mode]
            from_service = mock.create_autospec(Service)
            from_service.containers.return_value = [
                mock.Mock(id=container_id.split(':')[0], spec=Container)
                for container_id in container_ids
            ]
            service = Service(
                'test',
                volumes_from=[VolumeFromSpec(from_service, mode, 'service')],
                image='foo')

            assert service._get_volumes_from() == [container_ids[0]]

    def test_get_volumes_from_service_no_container(self):
        container_id = 'abababab'
        from_service = mock.create_autospec(Service)
        from_service.containers.return_value = []
        from_service.create_container.return_value = mock.Mock(
            id=container_id,
            spec=Container)
        service = Service(
            'test',
            image='foo',
            volumes_from=[VolumeFromSpec(from_service, 'rw', 'service')])

        assert service._get_volumes_from() == [container_id + ':rw']
        from_service.create_container.assert_called_once_with()

    def test_memory_swap_limit(self):
        self.mock_client.create_host_config.return_value = {}

        service = Service(
            name='foo',
            image='foo',
            hostname='name',
            client=self.mock_client,
            mem_limit=1000000000,
            memswap_limit=2000000000)
        service._get_container_create_options({'some': 'overrides'}, 1)

        assert self.mock_client.create_host_config.called
        assert self.mock_client.create_host_config.call_args[1]['mem_limit'] == 1000000000
        assert self.mock_client.create_host_config.call_args[1]['memswap_limit'] == 2000000000

    def test_self_reference_external_link(self):
        service = Service(
            name='foo',
            external_links=['default_foo_1']
        )
        with pytest.raises(DependencyError):
            service.get_container_name('foo', 1)

    def test_mem_reservation(self):
        self.mock_client.create_host_config.return_value = {}

        service = Service(
            name='foo',
            image='foo',
            hostname='name',
            client=self.mock_client,
            mem_reservation='512m'
        )
        service._get_container_create_options({'some': 'overrides'}, 1)
        assert self.mock_client.create_host_config.called is True
        assert self.mock_client.create_host_config.call_args[1]['mem_reservation'] == '512m'

    def test_cgroup_parent(self):
        self.mock_client.create_host_config.return_value = {}

        service = Service(
            name='foo',
            image='foo',
            hostname='name',
            client=self.mock_client,
            cgroup_parent='test')
        service._get_container_create_options({'some': 'overrides'}, 1)

        assert self.mock_client.create_host_config.called
        assert self.mock_client.create_host_config.call_args[1]['cgroup_parent'] == 'test'

    def test_log_opt(self):
        self.mock_client.create_host_config.return_value = {}

        log_opt = {'syslog-address': 'tcp://192.168.0.42:123'}
        logging = {'driver': 'syslog', 'options': log_opt}
        service = Service(
            name='foo',
            image='foo',
            hostname='name',
            client=self.mock_client,
            log_driver='syslog',
            logging=logging)
        service._get_container_create_options({'some': 'overrides'}, 1)

        assert self.mock_client.create_host_config.called
        assert self.mock_client.create_host_config.call_args[1]['log_config'] == {
            'Type': 'syslog', 'Config': {'syslog-address': 'tcp://192.168.0.42:123'}
        }

    def test_stop_grace_period(self):
        self.mock_client.api_version = '1.25'
        self.mock_client.create_host_config.return_value = {}
        service = Service(
            'foo',
            image='foo',
            client=self.mock_client,
            stop_grace_period="1m35s")
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        assert opts['stop_timeout'] == 95

    def test_split_domainname_none(self):
        service = Service(
            'foo',
            image='foo',
            hostname='name.domain.tld',
            client=self.mock_client)
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        assert opts['hostname'] == 'name.domain.tld', 'hostname'
        assert not ('domainname' in opts), 'domainname'

    def test_split_domainname_fqdn(self):
        self.mock_client.api_version = '1.22'
        service = Service(
            'foo',
            hostname='name.domain.tld',
            image='foo',
            client=self.mock_client)
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        assert opts['hostname'] == 'name', 'hostname'
        assert opts['domainname'] == 'domain.tld', 'domainname'

    def test_split_domainname_both(self):
        self.mock_client.api_version = '1.22'
        service = Service(
            'foo',
            hostname='name',
            image='foo',
            domainname='domain.tld',
            client=self.mock_client)
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        assert opts['hostname'] == 'name', 'hostname'
        assert opts['domainname'] == 'domain.tld', 'domainname'

    def test_split_domainname_weird(self):
        self.mock_client.api_version = '1.22'
        service = Service(
            'foo',
            hostname='name.sub',
            domainname='domain.tld',
            image='foo',
            client=self.mock_client)
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        assert opts['hostname'] == 'name.sub', 'hostname'
        assert opts['domainname'] == 'domain.tld', 'domainname'

    def test_no_default_hostname_when_not_using_networking(self):
        service = Service(
            'foo',
            image='foo',
            use_networking=False,
            client=self.mock_client,
        )
        opts = service._get_container_create_options({'image': 'foo'}, 1)
        assert opts.get('hostname') is None

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
            one_off=OneOffFilter.only)
        assert opts['name'] == name

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
            image_config={'ContainerConfig': {}}
        )
        prev_container.full_slug = 'abcdefff1234'
        prev_container.get.return_value = None

        opts = service._get_container_create_options(
            {}, 1, previous_container=prev_container
        )

        assert service.options['labels'] == labels
        assert service.options['environment'] == environment

        assert opts['labels'][LABEL_CONFIG_HASH] == \
            '689149e6041a85f6fb4945a2146a497ed43c8a5cbd8991753d875b165f1b4de4'
        assert opts['environment'] == ['also=real']

    def test_get_container_create_options_sets_affinity_with_binds(self):
        service = Service(
            'foo',
            image='foo',
            client=self.mock_client,
        )
        self.mock_client.inspect_image.return_value = {'Id': 'abcd'}
        prev_container = mock.Mock(
            id='ababab',
            image_config={'ContainerConfig': {'Volumes': ['/data']}})

        def container_get(key):
            return {
                'Mounts': [
                    {
                        'Destination': '/data',
                        'Source': '/some/path',
                        'Name': 'abab1234',
                    },
                ]
            }.get(key, None)

        prev_container.get.side_effect = container_get
        prev_container.full_slug = 'abcdefff1234'

        opts = service._get_container_create_options(
            {},
            1,
            previous_container=prev_container
        )

        assert opts['environment'] == ['affinity:container==ababab']

    def test_get_container_create_options_no_affinity_without_binds(self):
        service = Service('foo', image='foo', client=self.mock_client)
        self.mock_client.inspect_image.return_value = {'Id': 'abcd'}
        prev_container = mock.Mock(
            id='ababab',
            image_config={'ContainerConfig': {}})
        prev_container.get.return_value = None
        prev_container.full_slug = 'abcdefff1234'

        opts = service._get_container_create_options(
            {},
            1,
            previous_container=prev_container)
        assert opts['environment'] == []

    def test_get_container_not_found(self):
        self.mock_client.containers.return_value = []
        service = Service('foo', client=self.mock_client, image='foo')

        with pytest.raises(ValueError):
            service.get_container()

    @mock.patch('compose.service.Container', autospec=True)
    def test_get_container(self, mock_container_class):
        container_dict = dict(Name='default_foo_2_bdfa3ed91e2c')
        self.mock_client.containers.return_value = [container_dict]
        service = Service('foo', image='foo', client=self.mock_client)

        container = service.get_container(number=2)
        assert container == mock_container_class.from_ps.return_value
        mock_container_class.from_ps.assert_called_once_with(
            self.mock_client, container_dict)

    @mock.patch('compose.service.log', autospec=True)
    def test_pull_image(self, mock_log):
        service = Service('foo', client=self.mock_client, image='someimage:sometag')
        service.pull()
        self.mock_client.pull.assert_called_once_with(
            'someimage',
            tag='sometag',
            stream=True,
            platform=None)
        mock_log.info.assert_called_once_with('Pulling foo (someimage:sometag)...')

    def test_pull_image_no_tag(self):
        service = Service('foo', client=self.mock_client, image='ababab')
        service.pull()
        self.mock_client.pull.assert_called_once_with(
            'ababab',
            tag='latest',
            stream=True,
            platform=None)

    @mock.patch('compose.service.log', autospec=True)
    def test_pull_image_digest(self, mock_log):
        service = Service('foo', client=self.mock_client, image='someimage@sha256:1234')
        service.pull()
        self.mock_client.pull.assert_called_once_with(
            'someimage',
            tag='sha256:1234',
            stream=True,
            platform=None)
        mock_log.info.assert_called_once_with('Pulling foo (someimage@sha256:1234)...')

    @mock.patch('compose.service.log', autospec=True)
    def test_pull_image_with_platform(self, mock_log):
        self.mock_client.api_version = '1.35'
        service = Service(
            'foo', client=self.mock_client, image='someimage:sometag', platform='windows/x86_64'
        )
        service.pull()
        assert self.mock_client.pull.call_count == 1
        call_args = self.mock_client.pull.call_args
        assert call_args[1]['platform'] == 'windows/x86_64'

    @mock.patch('compose.service.log', autospec=True)
    def test_pull_image_with_platform_unsupported_api(self, mock_log):
        self.mock_client.api_version = '1.33'
        service = Service(
            'foo', client=self.mock_client, image='someimage:sometag', platform='linux/arm'
        )
        with pytest.raises(OperationFailedError):
            service.pull()

    def test_pull_image_with_default_platform(self):
        self.mock_client.api_version = '1.35'

        service = Service(
            'foo', client=self.mock_client, image='someimage:sometag',
            default_platform='linux'
        )
        assert service.platform == 'linux'
        service.pull()

        assert self.mock_client.pull.call_count == 1
        call_args = self.mock_client.pull.call_args
        assert call_args[1]['platform'] == 'linux'

    @mock.patch('compose.service.Container', autospec=True)
    def test_recreate_container(self, _):
        mock_container = mock.create_autospec(Container)
        mock_container.full_slug = 'abcdefff1234'
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
        mock_container.full_slug = 'abcdefff1234'
        self.mock_client.inspect_image.return_value = {'Id': 'abc123'}
        service = Service('foo', client=self.mock_client, image='someimage')
        service.recreate_container(mock_container, timeout=1)

        mock_container.stop.assert_called_once_with(timeout=1)

    def test_parse_repository_tag(self):
        assert parse_repository_tag("root") == ("root", "", ":")
        assert parse_repository_tag("root:tag") == ("root", "tag", ":")
        assert parse_repository_tag("user/repo") == ("user/repo", "", ":")
        assert parse_repository_tag("user/repo:tag") == ("user/repo", "tag", ":")
        assert parse_repository_tag("url:5000/repo") == ("url:5000/repo", "", ":")
        assert parse_repository_tag("url:5000/repo:tag") == ("url:5000/repo", "tag", ":")
        assert parse_repository_tag("root@sha256:digest") == ("root", "sha256:digest", "@")
        assert parse_repository_tag("user/repo@sha256:digest") == ("user/repo", "sha256:digest", "@")
        assert parse_repository_tag("url:5000/repo@sha256:digest") == (
            "url:5000/repo", "sha256:digest", "@"
        )

    def test_create_container(self):
        service = Service('foo', client=self.mock_client, build={'context': '.'})
        self.mock_client.inspect_image.side_effect = [
            NoSuchImageError,
            {'Id': 'abc123'},
        ]
        self.mock_client.build.return_value = [
            '{"stream": "Successfully built abcd"}',
        ]

        with mock.patch('compose.service.log', autospec=True) as mock_log:
            service.create_container()
            assert mock_log.warning.called
            _, args, _ = mock_log.warning.mock_calls[0]
            assert 'was built because it did not already exist' in args[0]

        assert self.mock_client.build.call_count == 1
        assert self.mock_client.build.call_args[1]['tag'] == 'default_foo'

    def test_create_container_binary_string_error(self):
        service = Service('foo', client=self.mock_client, build={'context': '.'})
        service.image = lambda: {'Id': 'abc123'}

        self.mock_client.create_container.side_effect = APIError(None,
                                                                 None,
                                                                 b"Test binary string explanation")
        with pytest.raises(OperationFailedError) as ex:
            service.create_container()

        assert ex.value.msg == "Cannot create container for service foo: Test binary string explanation"

    def test_start_binary_string_error(self):
        service = Service('foo', client=self.mock_client)
        container = Container(self.mock_client, {'Id': 'abc123'})

        self.mock_client.start.side_effect = APIError(None,
                                                      None,
                                                      b"Test binary string explanation with "
                                                      b"driver failed programming external "
                                                      b"connectivity")
        with mock.patch('compose.service.log', autospec=True) as mock_log:
            with pytest.raises(OperationFailedError) as ex:
                service.start_container(container)

        assert ex.value.msg == "Cannot start service foo: " \
                               "Test binary string explanation " \
                               "with driver failed programming external connectivity"
        mock_log.warn.assert_called_once_with("Host is already in use by another container")

    def test_ensure_image_exists_no_build(self):
        service = Service('foo', client=self.mock_client, build={'context': '.'})
        self.mock_client.inspect_image.return_value = {'Id': 'abc123'}

        service.ensure_image_exists(do_build=BuildAction.skip)
        assert not self.mock_client.build.called

    def test_ensure_image_exists_no_build_but_needs_build(self):
        service = Service('foo', client=self.mock_client, build={'context': '.'})
        self.mock_client.inspect_image.side_effect = NoSuchImageError
        with pytest.raises(NeedsBuildError):
            service.ensure_image_exists(do_build=BuildAction.skip)

    def test_ensure_image_exists_force_build(self):
        service = Service('foo', client=self.mock_client, build={'context': '.'})
        self.mock_client.inspect_image.return_value = {'Id': 'abc123'}
        self.mock_client.build.return_value = [
            '{"stream": "Successfully built abcd"}',
        ]

        with mock.patch('compose.service.log', autospec=True) as mock_log:
            service.ensure_image_exists(do_build=BuildAction.force)

        assert not mock_log.warning.called
        assert self.mock_client.build.call_count == 1
        self.mock_client.build.call_args[1]['tag'] == 'default_foo'

    def test_build_does_not_pull(self):
        self.mock_client.build.return_value = [
            b'{"stream": "Successfully built 12345"}',
        ]

        service = Service('foo', client=self.mock_client, build={'context': '.'})
        service.build()

        assert self.mock_client.build.call_count == 1
        assert not self.mock_client.build.call_args[1]['pull']

    def test_build_with_platform(self):
        self.mock_client.api_version = '1.35'
        self.mock_client.build.return_value = [
            b'{"stream": "Successfully built 12345"}',
        ]

        service = Service('foo', client=self.mock_client, build={'context': '.'}, platform='linux')
        service.build()

        assert self.mock_client.build.call_count == 1
        call_args = self.mock_client.build.call_args
        assert call_args[1]['platform'] == 'linux'

    def test_build_with_default_platform(self):
        self.mock_client.api_version = '1.35'
        self.mock_client.build.return_value = [
            b'{"stream": "Successfully built 12345"}',
        ]

        service = Service(
            'foo', client=self.mock_client, build={'context': '.'},
            default_platform='linux'
        )
        assert service.platform == 'linux'
        service.build()

        assert self.mock_client.build.call_count == 1
        call_args = self.mock_client.build.call_args
        assert call_args[1]['platform'] == 'linux'

    def test_service_platform_precedence(self):
        self.mock_client.api_version = '1.35'

        service = Service(
            'foo', client=self.mock_client, platform='linux/arm',
            default_platform='osx'
        )
        assert service.platform == 'linux/arm'

    def test_service_ignore_default_platform_with_unsupported_api(self):
        self.mock_client.api_version = '1.32'
        self.mock_client.build.return_value = [
            b'{"stream": "Successfully built 12345"}',
        ]

        service = Service(
            'foo', client=self.mock_client, default_platform='windows', build={'context': '.'}
        )
        assert service.platform is None
        service.build()
        assert self.mock_client.build.call_count == 1
        call_args = self.mock_client.build.call_args
        assert call_args[1]['platform'] is None

    def test_build_with_override_build_args(self):
        self.mock_client.build.return_value = [
            b'{"stream": "Successfully built 12345"}',
        ]

        build_args = {
            'arg1': 'arg1_new_value',
        }
        service = Service('foo', client=self.mock_client,
                          build={'context': '.', 'args': {'arg1': 'arg1', 'arg2': 'arg2'}})
        service.build(build_args_override=build_args)

        called_build_args = self.mock_client.build.call_args[1]['buildargs']

        assert called_build_args['arg1'] == build_args['arg1']
        assert called_build_args['arg2'] == 'arg2'

    def test_build_with_isolation_from_service_config(self):
        self.mock_client.build.return_value = [
            b'{"stream": "Successfully built 12345"}',
        ]

        service = Service('foo', client=self.mock_client, build={'context': '.'}, isolation='hyperv')
        service.build()

        assert self.mock_client.build.call_count == 1
        called_build_args = self.mock_client.build.call_args[1]
        assert called_build_args['isolation'] == 'hyperv'

    def test_build_isolation_from_build_override_service_config(self):
        self.mock_client.build.return_value = [
            b'{"stream": "Successfully built 12345"}',
        ]

        service = Service(
            'foo', client=self.mock_client, build={'context': '.', 'isolation': 'default'},
            isolation='hyperv'
        )
        service.build()

        assert self.mock_client.build.call_count == 1
        called_build_args = self.mock_client.build.call_args[1]
        assert called_build_args['isolation'] == 'default'

    def test_config_dict(self):
        self.mock_client.inspect_image.return_value = {'Id': 'abcd'}
        service = Service(
            'foo',
            image='example.com/foo',
            client=self.mock_client,
            network_mode=ServiceNetworkMode(Service('other')),
            networks={'default': None},
            links=[(Service('one'), 'one')],
            volumes_from=[VolumeFromSpec(Service('two'), 'rw', 'service')])

        config_dict = service.config_dict()
        expected = {
            'image_id': 'abcd',
            'options': {'image': 'example.com/foo'},
            'links': [('one', 'one')],
            'net': 'other',
            'secrets': [],
            'networks': {'default': None},
            'volumes_from': [('two', 'rw')],
        }
        assert config_dict == expected

    def test_config_dict_with_network_mode_from_container(self):
        self.mock_client.inspect_image.return_value = {'Id': 'abcd'}
        container = Container(
            self.mock_client,
            {'Id': 'aaabbb', 'Name': '/foo_1'})
        service = Service(
            'foo',
            image='example.com/foo',
            client=self.mock_client,
            network_mode=ContainerNetworkMode(container))

        config_dict = service.config_dict()
        expected = {
            'image_id': 'abcd',
            'options': {'image': 'example.com/foo'},
            'links': [],
            'networks': {},
            'secrets': [],
            'net': 'aaabbb',
            'volumes_from': [],
        }
        assert config_dict == expected

    def test_config_hash_matches_label(self):
        self.mock_client.inspect_image.return_value = {'Id': 'abcd'}
        service = Service(
            'foo',
            image='example.com/foo',
            client=self.mock_client,
            network_mode=NetworkMode('bridge'),
            networks={'bridge': {}, 'net2': {}},
            links=[(Service('one', client=self.mock_client), 'one')],
            volumes_from=[VolumeFromSpec(Service('two', client=self.mock_client), 'rw', 'service')],
            volumes=[VolumeSpec('/ext', '/int', 'ro')],
            build={'context': 'some/random/path'},
        )
        config_hash = service.config_hash

        for api_version in set(API_VERSIONS.values()):
            self.mock_client.api_version = api_version
            assert service._get_container_create_options(
                {}, 1
            )['labels'][LABEL_CONFIG_HASH] == config_hash

    def test_remove_image_none(self):
        web = Service('web', image='example', client=self.mock_client)
        assert not web.remove_image(ImageType.none)
        assert not self.mock_client.remove_image.called

    def test_remove_image_local_with_image_name_doesnt_remove(self):
        web = Service('web', image='example', client=self.mock_client)
        assert not web.remove_image(ImageType.local)
        assert not self.mock_client.remove_image.called

    def test_remove_image_local_without_image_name_does_remove(self):
        web = Service('web', build='.', client=self.mock_client)
        assert web.remove_image(ImageType.local)
        self.mock_client.remove_image.assert_called_once_with(web.image_name)

    def test_remove_image_all_does_remove(self):
        web = Service('web', image='example', client=self.mock_client)
        assert web.remove_image(ImageType.all)
        self.mock_client.remove_image.assert_called_once_with(web.image_name)

    def test_remove_image_with_error(self):
        self.mock_client.remove_image.side_effect = error = APIError(
            message="testing",
            response={},
            explanation="Boom")

        web = Service('web', image='example', client=self.mock_client)
        with mock.patch('compose.service.log', autospec=True) as mock_log:
            assert not web.remove_image(ImageType.all)
        mock_log.error.assert_called_once_with(
            "Failed to remove image for service %s: %s", web.name, error)

    def test_remove_non_existing_image(self):
        self.mock_client.remove_image.side_effect = ImageNotFound('image not found')
        web = Service('web', image='example', client=self.mock_client)
        with mock.patch('compose.service.log', autospec=True) as mock_log:
            assert not web.remove_image(ImageType.all)
        mock_log.warning.assert_called_once_with("Image %s not found.", web.image_name)

    def test_specifies_host_port_with_no_ports(self):
        service = Service(
            'foo',
            image='foo')
        assert not service.specifies_host_port()

    def test_specifies_host_port_with_container_port(self):
        service = Service(
            'foo',
            image='foo',
            ports=["2000"])
        assert not service.specifies_host_port()

    def test_specifies_host_port_with_host_port(self):
        service = Service(
            'foo',
            image='foo',
            ports=["1000:2000"])
        assert service.specifies_host_port()

    def test_specifies_host_port_with_host_ip_no_port(self):
        service = Service(
            'foo',
            image='foo',
            ports=["127.0.0.1::2000"])
        assert not service.specifies_host_port()

    def test_specifies_host_port_with_host_ip_and_port(self):
        service = Service(
            'foo',
            image='foo',
            ports=["127.0.0.1:1000:2000"])
        assert service.specifies_host_port()

    def test_specifies_host_port_with_container_port_range(self):
        service = Service(
            'foo',
            image='foo',
            ports=["2000-3000"])
        assert not service.specifies_host_port()

    def test_specifies_host_port_with_host_port_range(self):
        service = Service(
            'foo',
            image='foo',
            ports=["1000-2000:2000-3000"])
        assert service.specifies_host_port()

    def test_specifies_host_port_with_host_ip_no_port_range(self):
        service = Service(
            'foo',
            image='foo',
            ports=["127.0.0.1::2000-3000"])
        assert not service.specifies_host_port()

    def test_specifies_host_port_with_host_ip_and_port_range(self):
        service = Service(
            'foo',
            image='foo',
            ports=["127.0.0.1:1000-2000:2000-3000"])
        assert service.specifies_host_port()

    def test_image_name_from_config(self):
        image_name = 'example/web:mytag'
        service = Service('foo', image=image_name)
        assert service.image_name == image_name

    def test_image_name_default(self):
        service = Service('foo', project='testing')
        assert service.image_name == 'testing_foo'

    @mock.patch('compose.service.log', autospec=True)
    def test_only_log_warning_when_host_ports_clash(self, mock_log):
        self.mock_client.inspect_image.return_value = {'Id': 'abcd'}
        ParallelStreamWriter.instance = None
        name = 'foo'
        service = Service(
            name,
            client=self.mock_client,
            ports=["8080:80"])

        service.scale(0)
        assert not mock_log.warning.called

        service.scale(1)
        assert not mock_log.warning.called

        service.scale(2)
        mock_log.warning.assert_called_once_with(
            'The "{}" service specifies a port on the host. If multiple containers '
            'for this service are created on a single host, the port will clash.'.format(name))

    def test_parse_proxy_config(self):
        default_proxy_config = {
            'httpProxy': 'http://proxy.mycorp.com:3128',
            'httpsProxy': 'https://user:password@proxy.mycorp.com:3129',
            'ftpProxy': 'http://ftpproxy.mycorp.com:21',
            'noProxy': '*.intra.mycorp.com',
        }

        self.mock_client.base_url = 'http+docker://localunixsocket'
        self.mock_client._general_configs = {
            'proxies': {
                'default': default_proxy_config,
            }
        }

        service = Service('foo', client=self.mock_client)

        assert service._parse_proxy_config() == {
            'HTTP_PROXY': default_proxy_config['httpProxy'],
            'http_proxy': default_proxy_config['httpProxy'],
            'HTTPS_PROXY': default_proxy_config['httpsProxy'],
            'https_proxy': default_proxy_config['httpsProxy'],
            'FTP_PROXY': default_proxy_config['ftpProxy'],
            'ftp_proxy': default_proxy_config['ftpProxy'],
            'NO_PROXY': default_proxy_config['noProxy'],
            'no_proxy': default_proxy_config['noProxy'],
        }

    def test_parse_proxy_config_per_host(self):
        default_proxy_config = {
            'httpProxy': 'http://proxy.mycorp.com:3128',
            'httpsProxy': 'https://user:password@proxy.mycorp.com:3129',
            'ftpProxy': 'http://ftpproxy.mycorp.com:21',
            'noProxy': '*.intra.mycorp.com',
        }
        host_specific_proxy_config = {
            'httpProxy': 'http://proxy.example.com:3128',
            'httpsProxy': 'https://user:password@proxy.example.com:3129',
            'ftpProxy': 'http://ftpproxy.example.com:21',
            'noProxy': '*.intra.example.com'
        }

        self.mock_client.base_url = 'http+docker://localunixsocket'
        self.mock_client._general_configs = {
            'proxies': {
                'default': default_proxy_config,
                'tcp://example.docker.com:2376': host_specific_proxy_config,
            }
        }

        service = Service('foo', client=self.mock_client)

        assert service._parse_proxy_config() == {
            'HTTP_PROXY': default_proxy_config['httpProxy'],
            'http_proxy': default_proxy_config['httpProxy'],
            'HTTPS_PROXY': default_proxy_config['httpsProxy'],
            'https_proxy': default_proxy_config['httpsProxy'],
            'FTP_PROXY': default_proxy_config['ftpProxy'],
            'ftp_proxy': default_proxy_config['ftpProxy'],
            'NO_PROXY': default_proxy_config['noProxy'],
            'no_proxy': default_proxy_config['noProxy'],
        }

        self.mock_client._original_base_url = 'tcp://example.docker.com:2376'

        assert service._parse_proxy_config() == {
            'HTTP_PROXY': host_specific_proxy_config['httpProxy'],
            'http_proxy': host_specific_proxy_config['httpProxy'],
            'HTTPS_PROXY': host_specific_proxy_config['httpsProxy'],
            'https_proxy': host_specific_proxy_config['httpsProxy'],
            'FTP_PROXY': host_specific_proxy_config['ftpProxy'],
            'ftp_proxy': host_specific_proxy_config['ftpProxy'],
            'NO_PROXY': host_specific_proxy_config['noProxy'],
            'no_proxy': host_specific_proxy_config['noProxy'],
        }

    def test_build_service_with_proxy_config(self):
        default_proxy_config = {
            'httpProxy': 'http://proxy.mycorp.com:3128',
            'httpsProxy': 'https://user:password@proxy.example.com:3129',
        }
        buildargs = {
            'HTTPS_PROXY': 'https://rdcf.th08.jp:8911',
            'https_proxy': 'https://rdcf.th08.jp:8911',
        }
        self.mock_client._general_configs = {
            'proxies': {
                'default': default_proxy_config,
            }
        }
        self.mock_client.base_url = 'http+docker://localunixsocket'
        self.mock_client.build.return_value = [
            b'{"stream": "Successfully built 12345"}',
        ]

        service = Service('foo', client=self.mock_client, build={'context': '.', 'args': buildargs})
        service.build()

        assert self.mock_client.build.call_count == 1
        assert self.mock_client.build.call_args[1]['buildargs'] == {
            'HTTP_PROXY': default_proxy_config['httpProxy'],
            'http_proxy': default_proxy_config['httpProxy'],
            'HTTPS_PROXY': buildargs['HTTPS_PROXY'],
            'https_proxy': buildargs['HTTPS_PROXY'],
        }

    def test_get_create_options_with_proxy_config(self):
        default_proxy_config = {
            'httpProxy': 'http://proxy.mycorp.com:3128',
            'httpsProxy': 'https://user:password@proxy.mycorp.com:3129',
            'ftpProxy': 'http://ftpproxy.mycorp.com:21',
        }
        self.mock_client._general_configs = {
            'proxies': {
                'default': default_proxy_config,
            }
        }
        self.mock_client.base_url = 'http+docker://localunixsocket'

        override_options = {
            'environment': {
                'FTP_PROXY': 'ftp://xdge.exo.au:21',
                'ftp_proxy': 'ftp://xdge.exo.au:21',
            }
        }
        environment = {
            'HTTPS_PROXY': 'https://rdcf.th08.jp:8911',
            'https_proxy': 'https://rdcf.th08.jp:8911',
        }

        service = Service('foo', client=self.mock_client, environment=environment)

        create_opts = service._get_container_create_options(override_options, 1)
        assert set(create_opts['environment']) == set(format_environment({
            'HTTP_PROXY': default_proxy_config['httpProxy'],
            'http_proxy': default_proxy_config['httpProxy'],
            'HTTPS_PROXY': environment['HTTPS_PROXY'],
            'https_proxy': environment['HTTPS_PROXY'],
            'FTP_PROXY': override_options['environment']['FTP_PROXY'],
            'ftp_proxy': override_options['environment']['FTP_PROXY'],
        }))

    def test_create_when_removed_containers_are_listed(self):
        # This is aimed at simulating a race between the API call to list the
        # containers, and the ones to inspect each of the listed containers.
        # It can happen that a container has been removed after we listed it.

        # containers() returns a container that is about to be removed
        self.mock_client.containers.return_value = [
            {'Id': 'rm_cont_id', 'Name': 'rm_cont', 'Image': 'img_id'},
        ]

        # inspect_container() will raise a NotFound when trying to inspect
        # rm_cont_id, which at this point has been removed
        def inspect(name):
            if name == 'rm_cont_id':
                raise NotFound(message='Not Found')

            if name == 'new_cont_id':
                return {'Id': 'new_cont_id'}

            raise NotImplementedError("incomplete mock")

        self.mock_client.inspect_container.side_effect = inspect

        self.mock_client.inspect_image.return_value = {'Id': 'imageid'}

        self.mock_client.create_container.return_value = {'Id': 'new_cont_id'}

        # We should nonetheless be able to create a new container
        service = Service('foo', client=self.mock_client)

        assert service.create_container().id == 'new_cont_id'

    def test_build_volume_options_duplicate_binds(self):
        self.mock_client.api_version = '1.29'  # Trigger 3.2 format workaround
        service = Service('foo', client=self.mock_client)
        ctnr_opts, override_opts = service._build_container_volume_options(
            previous_container=None,
            container_options={
                'volumes': [
                    MountSpec.parse({'source': 'vol', 'target': '/data', 'type': 'volume'}),
                    VolumeSpec.parse('vol:/data:rw'),
                ],
                'environment': {},
            },
            override_options={},
        )
        assert 'binds' in override_opts
        assert len(override_opts['binds']) == 1
        assert override_opts['binds'][0] == 'vol:/data:rw'

    def test_volumes_order_is_preserved(self):
        service = Service('foo', client=self.mock_client)
        volumes = [
            VolumeSpec.parse(cfg) for cfg in [
                '/v{0}:/v{0}:rw'.format(i) for i in range(6)
            ]
        ]
        ctnr_opts, override_opts = service._build_container_volume_options(
            previous_container=None,
            container_options={
                'volumes': volumes,
                'environment': {},
            },
            override_options={},
        )
        assert override_opts['binds'] == [vol.repr() for vol in volumes]


class TestServiceNetwork(unittest.TestCase):
    def setUp(self):
        self.mock_client = mock.create_autospec(docker.APIClient)
        self.mock_client.api_version = DEFAULT_DOCKER_API_VERSION
        self.mock_client._general_configs = {}

    def test_connect_container_to_networks_short_aliase_exists(self):
        service = Service(
            'db',
            self.mock_client,
            'myproject',
            image='foo',
            networks={'project_default': {}})
        container = Container(
            None,
            {
                'Id': 'abcdef',
                'NetworkSettings': {
                    'Networks': {
                        'project_default': {
                            'Aliases': ['analias', 'abcdef'],
                        },
                    },
                },
            },
            True)
        service.connect_container_to_networks(container)

        assert not self.mock_client.disconnect_container_from_network.call_count
        assert not self.mock_client.connect_container_to_network.call_count


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
    def setUp(self):
        self.mock_client = mock.create_autospec(docker.APIClient)
        self.mock_client.api_version = DEFAULT_DOCKER_API_VERSION
        self.mock_client._general_configs = {}

    def test_network_mode(self):
        network_mode = NetworkMode('host')
        assert network_mode.id == 'host'
        assert network_mode.mode == 'host'
        assert network_mode.service_name is None

    def test_network_mode_container(self):
        container_id = 'abcd'
        network_mode = ContainerNetworkMode(Container(None, {'Id': container_id}))
        assert network_mode.id == container_id
        assert network_mode.mode == 'container:' + container_id
        assert network_mode.service_name is None

    def test_network_mode_service(self):
        container_id = 'bbbb'
        service_name = 'web'
        self.mock_client.containers.return_value = [
            {'Id': container_id, 'Name': container_id, 'Image': 'abcd'},
        ]

        service = Service(name=service_name, client=self.mock_client)
        network_mode = ServiceNetworkMode(service)

        assert network_mode.id == service_name
        assert network_mode.mode == 'container:' + container_id
        assert network_mode.service_name == service_name

    def test_network_mode_service_no_containers(self):
        service_name = 'web'
        self.mock_client.containers.return_value = []

        service = Service(name=service_name, client=self.mock_client)
        network_mode = ServiceNetworkMode(service)

        assert network_mode.id == service_name
        assert network_mode.mode is None
        assert network_mode.service_name == service_name


class ServicePortsTest(unittest.TestCase):
    def test_formatted_ports(self):
        ports = [
            '3000',
            '0.0.0.0:4025-4030:23000-23005',
            ServicePort(6000, None, None, None, None),
            ServicePort(8080, 8080, None, None, None),
            ServicePort('20000', '20000', 'udp', 'ingress', None),
            ServicePort(30000, '30000', 'tcp', None, '127.0.0.1'),
        ]
        formatted = formatted_ports(ports)
        assert ports[0] in formatted
        assert ports[1] in formatted
        assert '6000/tcp' in formatted
        assert '8080:8080/tcp' in formatted
        assert '20000:20000/udp' in formatted
        assert '127.0.0.1:30000:30000/tcp' in formatted


def build_mount(destination, source, mode='rw'):
    return {'Source': source, 'Destination': destination, 'Mode': mode}


class ServiceVolumesTest(unittest.TestCase):

    def setUp(self):
        self.mock_client = mock.create_autospec(docker.APIClient)
        self.mock_client.api_version = DEFAULT_DOCKER_API_VERSION
        self.mock_client._general_configs = {}

    def test_build_volume_binding(self):
        binding = build_volume_binding(VolumeSpec.parse('/outside:/inside', True))
        assert binding == ('/inside', '/outside:/inside:rw')

    def test_get_container_data_volumes(self):
        options = [VolumeSpec.parse(v) for v in [
            '/host/volume:/host/volume:ro',
            '/new/volume',
            '/existing/volume',
            'named:/named/vol',
            '/dev/tmpfs'
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
            'Mounts': [
                {
                    'Source': '/host/volume',
                    'Destination': '/host/volume',
                    'Mode': '',
                    'RW': True,
                    'Name': 'hostvolume',
                }, {
                    'Source': '/var/lib/docker/aaaaaaaa',
                    'Destination': '/existing/volume',
                    'Mode': '',
                    'RW': True,
                    'Name': 'existingvolume',
                }, {
                    'Source': '/var/lib/docker/bbbbbbbb',
                    'Destination': '/removed/volume',
                    'Mode': '',
                    'RW': True,
                    'Name': 'removedvolume',
                }, {
                    'Source': '/var/lib/docker/cccccccc',
                    'Destination': '/mnt/image/data',
                    'Mode': '',
                    'RW': True,
                    'Name': 'imagedata',
                },
            ]
        }, has_been_inspected=True)

        expected = [
            VolumeSpec.parse('existingvolume:/existing/volume:rw'),
            VolumeSpec.parse('imagedata:/mnt/image/data:rw'),
        ]

        volumes, _ = get_container_data_volumes(container, options, ['/dev/tmpfs'], [])
        assert sorted(volumes) == sorted(expected)

    def test_merge_volume_bindings(self):
        options = [
            VolumeSpec.parse(v, True) for v in [
                '/host/volume:/host/volume:ro',
                '/host/rw/volume:/host/rw/volume',
                '/new/volume',
                '/existing/volume',
                '/dev/tmpfs'
            ]
        ]

        self.mock_client.inspect_image.return_value = {
            'ContainerConfig': {'Volumes': {}}
        }

        previous_container = Container(self.mock_client, {
            'Id': 'cdefab',
            'Image': 'ababab',
            'Mounts': [{
                'Source': '/var/lib/docker/aaaaaaaa',
                'Destination': '/existing/volume',
                'Mode': '',
                'RW': True,
                'Name': 'existingvolume',
            }],
        }, has_been_inspected=True)

        expected = [
            '/host/volume:/host/volume:ro',
            '/host/rw/volume:/host/rw/volume:rw',
            'existingvolume:/existing/volume:rw',
        ]

        binds, affinity = merge_volume_bindings(options, ['/dev/tmpfs'], previous_container, [])
        assert sorted(binds) == sorted(expected)
        assert affinity == {'affinity:container': '=cdefab'}

    def test_mount_same_host_path_to_two_volumes(self):
        service = Service(
            'web',
            image='busybox',
            volumes=[
                VolumeSpec.parse('/host/path:/data1', True),
                VolumeSpec.parse('/host/path:/data2', True),
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

        assert set(self.mock_client.create_host_config.call_args[1]['binds']) == {'/host/path:/data1:rw',
                                                                                  '/host/path:/data2:rw'}

    def test_get_container_create_options_with_different_host_path_in_container_json(self):
        service = Service(
            'web',
            image='busybox',
            volumes=[VolumeSpec.parse('/host/path:/data')],
            client=self.mock_client,
        )
        volume_name = 'abcdefff1234'

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
            'Mounts': [
                {
                    'Destination': '/data',
                    'Source': '/mnt/sda1/host/path',
                    'Mode': '',
                    'RW': True,
                    'Driver': 'local',
                    'Name': volume_name,
                },
            ]
        }

        service._get_container_create_options(
            override_options={},
            number=1,
            previous_container=Container(self.mock_client, {'Id': '123123123'}),
        )

        assert (
            self.mock_client.create_host_config.call_args[1]['binds'] ==
            ['{}:/data:rw'.format(volume_name)]
        )

    def test_warn_on_masked_volume_no_warning_when_no_container_volumes(self):
        volumes_option = [VolumeSpec('/home/user', '/path', 'rw')]
        container_volumes = []
        service = 'service_name'

        with mock.patch('compose.service.log', autospec=True) as mock_log:
            warn_on_masked_volume(volumes_option, container_volumes, service)

        assert not mock_log.warning.called

    def test_warn_on_masked_volume_when_masked(self):
        volumes_option = [VolumeSpec('/home/user', '/path', 'rw')]
        container_volumes = [
            VolumeSpec('/var/lib/docker/path', '/path', 'rw'),
            VolumeSpec('/var/lib/docker/path', '/other', 'rw'),
        ]
        service = 'service_name'

        with mock.patch('compose.service.log', autospec=True) as mock_log:
            warn_on_masked_volume(volumes_option, container_volumes, service)

        mock_log.warning.assert_called_once_with(mock.ANY)

    def test_warn_on_masked_no_warning_with_same_path(self):
        volumes_option = [VolumeSpec('/home/user', '/path', 'rw')]
        container_volumes = [VolumeSpec('/home/user', '/path', 'rw')]
        service = 'service_name'

        with mock.patch('compose.service.log', autospec=True) as mock_log:
            warn_on_masked_volume(volumes_option, container_volumes, service)

        assert not mock_log.warning.called

    def test_warn_on_masked_no_warning_with_container_only_option(self):
        volumes_option = [VolumeSpec(None, '/path', 'rw')]
        container_volumes = [
            VolumeSpec('/var/lib/docker/volume/path', '/path', 'rw')
        ]
        service = 'service_name'

        with mock.patch('compose.service.log', autospec=True) as mock_log:
            warn_on_masked_volume(volumes_option, container_volumes, service)

        assert not mock_log.warning.called

    def test_create_with_special_volume_mode(self):
        self.mock_client.inspect_image.return_value = {'Id': 'imageid'}

        self.mock_client.create_container.return_value = {'Id': 'containerid'}

        volume = '/tmp:/foo:z'
        Service(
            'web',
            client=self.mock_client,
            image='busybox',
            volumes=[VolumeSpec.parse(volume, True)],
        ).create_container()

        assert self.mock_client.create_container.call_count == 1
        assert self.mock_client.create_host_config.call_args[1]['binds'] == [volume]


class ServiceSecretTest(unittest.TestCase):
    def setUp(self):
        self.mock_client = mock.create_autospec(docker.APIClient)
        self.mock_client.api_version = DEFAULT_DOCKER_API_VERSION
        self.mock_client._general_configs = {}

    def test_get_secret_volumes(self):
        secret1 = {
            'secret': ServiceSecret.parse({'source': 'secret1', 'target': 'b.txt'}),
            'file': 'a.txt'
        }
        service = Service(
            'web',
            client=self.mock_client,
            image='busybox',
            secrets=[secret1]
        )
        volumes = service.get_secret_volumes()

        assert volumes[0].source == secret1['file']
        assert volumes[0].target == '{}/{}'.format(SECRETS_PATH, secret1['secret'].target)

    def test_get_secret_volumes_abspath(self):
        secret1 = {
            'secret': ServiceSecret.parse({'source': 'secret1', 'target': '/d.txt'}),
            'file': 'c.txt'
        }
        service = Service(
            'web',
            client=self.mock_client,
            image='busybox',
            secrets=[secret1]
        )
        volumes = service.get_secret_volumes()

        assert volumes[0].source == secret1['file']
        assert volumes[0].target == secret1['secret'].target

    def test_get_secret_volumes_no_target(self):
        secret1 = {
            'secret': ServiceSecret.parse({'source': 'secret1'}),
            'file': 'c.txt'
        }
        service = Service(
            'web',
            client=self.mock_client,
            image='busybox',
            secrets=[secret1]
        )
        volumes = service.get_secret_volumes()

        assert volumes[0].source == secret1['file']
        assert volumes[0].target == '{}/{}'.format(SECRETS_PATH, secret1['secret'].source)


class RewriteBuildPathTest(unittest.TestCase):
    @mock.patch('compose.service.IS_WINDOWS_PLATFORM', True)
    def test_rewrite_url_no_prefix(self):
        urls = [
            'http://test.com',
            'https://test.com',
            'git://test.com',
            'github.com/test/test',
            'git@test.com',
        ]
        for u in urls:
            assert rewrite_build_path(u) == u

    @mock.patch('compose.service.IS_WINDOWS_PLATFORM', True)
    def test_rewrite_windows_path(self):
        assert rewrite_build_path('C:\\context') == WINDOWS_LONGPATH_PREFIX + 'C:\\context'
        assert rewrite_build_path(
            rewrite_build_path('C:\\context')
        ) == rewrite_build_path('C:\\context')

    @mock.patch('compose.service.IS_WINDOWS_PLATFORM', False)
    def test_rewrite_unix_path(self):
        assert rewrite_build_path('/context') == '/context'
