from __future__ import absolute_import
from __future__ import unicode_literals

import docker
import pytest

from .. import mock
from compose import bundle
from compose import service
from compose.cli.errors import UserError
from compose.config.config import Config
from compose.const import COMPOSEFILE_V2_0 as V2_0
from compose.service import NoSuchImageError


@pytest.fixture
def mock_service():
    return mock.create_autospec(
        service.Service,
        client=mock.create_autospec(docker.APIClient),
        options={})


def test_get_image_digest_exists(mock_service):
    mock_service.options['image'] = 'abcd'
    mock_service.image.return_value = {'RepoDigests': ['digest1']}
    digest = bundle.get_image_digest(mock_service)
    assert digest == 'digest1'


def test_get_image_digest_image_uses_digest(mock_service):
    mock_service.options['image'] = image_id = 'redis@sha256:digest'

    digest = bundle.get_image_digest(mock_service)
    assert digest == image_id
    assert not mock_service.image.called


def test_get_image_digest_from_repository(mock_service):
    mock_service.options['image'] = 'abcd'
    mock_service.image_name = 'abcd'
    mock_service.image.side_effect = NoSuchImageError(None)
    mock_service.get_image_registry_data.return_value = {'Descriptor': {'digest': 'digest'}}

    digest = bundle.get_image_digest(mock_service)
    assert digest == 'abcd@digest'


def test_get_image_digest_no_image(mock_service):
    with pytest.raises(UserError) as exc:
        bundle.get_image_digest(service.Service(name='theservice'))

    assert "doesn't define an image tag" in exc.exconly()


def test_push_image_with_saved_digest(mock_service):
    mock_service.options['build'] = '.'
    mock_service.options['image'] = image_id = 'abcd'
    mock_service.push.return_value = expected = 'sha256:thedigest'
    mock_service.image.return_value = {'RepoDigests': ['digest1']}

    digest = bundle.push_image(mock_service)
    assert digest == image_id + '@' + expected

    mock_service.push.assert_called_once_with()
    assert not mock_service.client.push.called


def test_push_image(mock_service):
    mock_service.options['build'] = '.'
    mock_service.options['image'] = image_id = 'abcd'
    mock_service.push.return_value = expected = 'sha256:thedigest'
    mock_service.image.return_value = {'RepoDigests': []}

    digest = bundle.push_image(mock_service)
    assert digest == image_id + '@' + expected

    mock_service.push.assert_called_once_with()
    mock_service.client.pull.assert_called_once_with(digest)


def test_to_bundle():
    image_digests = {'a': 'aaaa', 'b': 'bbbb'}
    services = [
        {'name': 'a', 'build': '.', },
        {'name': 'b', 'build': './b'},
    ]
    config = Config(
        version=V2_0,
        services=services,
        volumes={'special': {}},
        networks={'extra': {}},
        secrets={},
        configs={}
    )

    with mock.patch('compose.bundle.log.warning', autospec=True) as mock_log:
        output = bundle.to_bundle(config, image_digests)

    assert mock_log.mock_calls == [
        mock.call("Unsupported top level key 'networks' - ignoring"),
        mock.call("Unsupported top level key 'volumes' - ignoring"),
    ]

    assert output == {
        'Version': '0.1',
        'Services': {
            'a': {'Image': 'aaaa', 'Networks': ['default']},
            'b': {'Image': 'bbbb', 'Networks': ['default']},
        }
    }


def test_convert_service_to_bundle():
    name = 'theservice'
    image_digest = 'thedigest'
    service_dict = {
        'ports': ['80'],
        'expose': ['1234'],
        'networks': {'extra': {}},
        'command': 'foo',
        'entrypoint': 'entry',
        'environment': {'BAZ': 'ENV'},
        'build': '.',
        'working_dir': '/tmp',
        'user': 'root',
        'labels': {'FOO': 'LABEL'},
        'privileged': True,
    }

    with mock.patch('compose.bundle.log.warning', autospec=True) as mock_log:
        config = bundle.convert_service_to_bundle(name, service_dict, image_digest)

    mock_log.assert_called_once_with(
        "Unsupported key 'privileged' in services.theservice - ignoring")

    assert config == {
        'Image': image_digest,
        'Ports': [
            {'Protocol': 'tcp', 'Port': 80},
            {'Protocol': 'tcp', 'Port': 1234},
        ],
        'Networks': ['extra'],
        'Command': ['entry', 'foo'],
        'Env': ['BAZ=ENV'],
        'WorkingDir': '/tmp',
        'User': 'root',
        'Labels': {'FOO': 'LABEL'},
    }


def test_set_command_and_args_none():
    config = {}
    bundle.set_command_and_args(config, [], [])
    assert config == {}


def test_set_command_and_args_from_command():
    config = {}
    bundle.set_command_and_args(config, [], "echo ok")
    assert config == {'Args': ['echo', 'ok']}


def test_set_command_and_args_from_entrypoint():
    config = {}
    bundle.set_command_and_args(config, "echo entry", [])
    assert config == {'Command': ['echo', 'entry']}


def test_set_command_and_args_from_both():
    config = {}
    bundle.set_command_and_args(config, "echo entry", ["extra", "arg"])
    assert config == {'Command': ['echo', 'entry', "extra", "arg"]}


def test_make_service_networks_default():
    name = 'theservice'
    service_dict = {}

    with mock.patch('compose.bundle.log.warning', autospec=True) as mock_log:
        networks = bundle.make_service_networks(name, service_dict)

    assert not mock_log.called
    assert networks == ['default']


def test_make_service_networks():
    name = 'theservice'
    service_dict = {
        'networks': {
            'foo': {
                'aliases': ['one', 'two'],
            },
            'bar': {}
        },
    }

    with mock.patch('compose.bundle.log.warning', autospec=True) as mock_log:
        networks = bundle.make_service_networks(name, service_dict)

    mock_log.assert_called_once_with(
        "Unsupported key 'aliases' in services.theservice.networks.foo - ignoring")
    assert sorted(networks) == sorted(service_dict['networks'])


def test_make_port_specs():
    service_dict = {
        'expose': ['80', '500/udp'],
        'ports': [
            '400:80',
            '222',
            '127.0.0.1:8001:8001',
            '127.0.0.1:5000-5001:3000-3001'],
    }
    port_specs = bundle.make_port_specs(service_dict)
    assert port_specs == [
        {'Protocol': 'tcp', 'Port': 80},
        {'Protocol': 'tcp', 'Port': 222},
        {'Protocol': 'tcp', 'Port': 8001},
        {'Protocol': 'tcp', 'Port': 3000},
        {'Protocol': 'tcp', 'Port': 3001},
        {'Protocol': 'udp', 'Port': 500},
    ]


def test_make_port_spec_with_protocol():
    port_spec = bundle.make_port_spec("5000/udp")
    assert port_spec == {'Protocol': 'udp', 'Port': 5000}


def test_make_port_spec_default_protocol():
    port_spec = bundle.make_port_spec("50000")
    assert port_spec == {'Protocol': 'tcp', 'Port': 50000}
