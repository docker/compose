from __future__ import absolute_import
from __future__ import unicode_literals

import json
import logging

import six
from docker.utils import split_command
from docker.utils.ports import split_port

from .cli.errors import UserError
from .config.serialize import denormalize_config
from .network import get_network_defs_for_service
from .service import format_environment
from .service import NoSuchImageError
from .service import parse_repository_tag


log = logging.getLogger(__name__)


SERVICE_KEYS = {
    'working_dir': 'WorkingDir',
    'user': 'User',
    'labels': 'Labels',
}

IGNORED_KEYS = {'build'}

SUPPORTED_KEYS = {
    'image',
    'ports',
    'expose',
    'networks',
    'command',
    'environment',
    'entrypoint',
} | set(SERVICE_KEYS)

VERSION = '0.1'


class NeedsPush(Exception):
    def __init__(self, image_name):
        self.image_name = image_name


class NeedsPull(Exception):
    def __init__(self, image_name, service_name):
        self.image_name = image_name
        self.service_name = service_name


class MissingDigests(Exception):
    def __init__(self, needs_push, needs_pull):
        self.needs_push = needs_push
        self.needs_pull = needs_pull


def serialize_bundle(config, image_digests):
    return json.dumps(to_bundle(config, image_digests), indent=2, sort_keys=True)


def get_image_digests(project, allow_push=False):
    digests = {}
    needs_push = set()
    needs_pull = set()

    for service in project.services:
        try:
            digests[service.name] = get_image_digest(
                service,
                allow_push=allow_push,
            )
        except NeedsPush as e:
            needs_push.add(e.image_name)
        except NeedsPull as e:
            needs_pull.add(e.service_name)

    if needs_push or needs_pull:
        raise MissingDigests(needs_push, needs_pull)

    return digests


def get_image_digest(service, allow_push=False):
    if 'image' not in service.options:
        raise UserError(
            "Service '{s.name}' doesn't define an image tag. An image name is "
            "required to generate a proper image digest for the bundle. Specify "
            "an image repo and tag with the 'image' option.".format(s=service))

    _, _, separator = parse_repository_tag(service.options['image'])
    # Compose file already uses a digest, no lookup required
    if separator == '@':
        return service.options['image']

    digest = get_digest(service)

    if digest:
        return digest

    if 'build' not in service.options:
        raise NeedsPull(service.image_name, service.name)

    if not allow_push:
        raise NeedsPush(service.image_name)

    return push_image(service)


def get_digest(service):
    digest = None
    try:
        image = service.image()
        # TODO: pick a digest based on the image tag if there are multiple
        # digests
        if image['RepoDigests']:
            digest = image['RepoDigests'][0]
    except NoSuchImageError:
        try:
            # Fetch the image digest from the registry
            distribution = service.get_image_registry_data()

            if distribution['Descriptor']['digest']:
                digest = '{image_name}@{digest}'.format(
                    image_name=service.image_name,
                    digest=distribution['Descriptor']['digest']
                )
        except NoSuchImageError:
            raise UserError(
                "Digest not found for service '{service}'. "
                "Repository does not exist or may require 'docker login'"
                .format(service=service.name))
    return digest


def push_image(service):
    try:
        digest = service.push()
    except Exception:
        log.error(
            "Failed to push image for service '{s.name}'. Please use an "
            "image tag that can be pushed to a Docker "
            "registry.".format(s=service))
        raise

    if not digest:
        raise ValueError("Failed to get digest for %s" % service.name)

    repo, _, _ = parse_repository_tag(service.options['image'])
    identifier = '{repo}@{digest}'.format(repo=repo, digest=digest)

    # only do this if RepoDigests isn't already populated
    image = service.image()
    if not image['RepoDigests']:
        # Pull by digest so that image['RepoDigests'] is populated for next time
        # and we don't have to pull/push again
        service.client.pull(identifier)
        log.info("Stored digest for {}".format(service.image_name))

    return identifier


def to_bundle(config, image_digests):
    if config.networks:
        log.warning("Unsupported top level key 'networks' - ignoring")

    if config.volumes:
        log.warning("Unsupported top level key 'volumes' - ignoring")

    config = denormalize_config(config)

    return {
        'Version': VERSION,
        'Services': {
            name: convert_service_to_bundle(
                name,
                service_dict,
                image_digests[name],
            )
            for name, service_dict in config['services'].items()
        },
    }


def convert_service_to_bundle(name, service_dict, image_digest):
    container_config = {'Image': image_digest}

    for key, value in service_dict.items():
        if key in IGNORED_KEYS:
            continue

        if key not in SUPPORTED_KEYS:
            log.warning("Unsupported key '{}' in services.{} - ignoring".format(key, name))
            continue

        if key == 'environment':
            container_config['Env'] = format_environment({
                envkey: envvalue for envkey, envvalue in value.items()
                if envvalue
            })
            continue

        if key in SERVICE_KEYS:
            container_config[SERVICE_KEYS[key]] = value
            continue

    set_command_and_args(
        container_config,
        service_dict.get('entrypoint', []),
        service_dict.get('command', []))
    container_config['Networks'] = make_service_networks(name, service_dict)

    ports = make_port_specs(service_dict)
    if ports:
        container_config['Ports'] = ports

    return container_config


# See https://github.com/docker/swarmkit/blob/agent/exec/container/container.go#L95
def set_command_and_args(config, entrypoint, command):
    if isinstance(entrypoint, six.string_types):
        entrypoint = split_command(entrypoint)
    if isinstance(command, six.string_types):
        command = split_command(command)

    if entrypoint:
        config['Command'] = entrypoint + command
        return

    if command:
        config['Args'] = command


def make_service_networks(name, service_dict):
    networks = []

    for network_name, network_def in get_network_defs_for_service(service_dict).items():
        for key in network_def.keys():
            log.warning(
                "Unsupported key '{}' in services.{}.networks.{} - ignoring"
                .format(key, name, network_name))

        networks.append(network_name)

    return networks


def make_port_specs(service_dict):
    ports = []

    internal_ports = [
        internal_port
        for port_def in service_dict.get('ports', [])
        for internal_port in split_port(port_def)[0]
    ]

    internal_ports += service_dict.get('expose', [])

    for internal_port in internal_ports:
        spec = make_port_spec(internal_port)
        if spec not in ports:
            ports.append(spec)

    return ports


def make_port_spec(value):
    components = six.text_type(value).partition('/')
    return {
        'Protocol': components[2] or 'tcp',
        'Port': int(components[0]),
    }
