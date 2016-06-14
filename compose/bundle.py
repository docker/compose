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


def serialize_bundle(config, image_digests):
    if config.networks:
        log.warn("Unsupported top level key 'networks' - ignoring")

    if config.volumes:
        log.warn("Unsupported top level key 'volumes' - ignoring")

    return json.dumps(
        to_bundle(config, image_digests),
        indent=2,
        sort_keys=True,
    )


def get_image_digests(project):
    return {
        service.name: get_image_digest(service)
        for service in project.services
    }


def get_image_digest(service):
    if 'image' not in service.options:
        raise UserError(
            "Service '{s.name}' doesn't define an image tag. An image name is "
            "required to generate a proper image digest for the bundle. Specify "
            "an image repo and tag with the 'image' option.".format(s=service))

    repo, tag, separator = parse_repository_tag(service.options['image'])
    # Compose file already uses a digest, no lookup required
    if separator == '@':
        return service.options['image']

    try:
        image = service.image()
    except NoSuchImageError:
        action = 'build' if 'build' in service.options else 'pull'
        raise UserError(
            "Image not found for service '{service}'. "
            "You might need to run `docker-compose {action} {service}`."
            .format(service=service.name, action=action))

    if image['RepoDigests']:
        # TODO: pick a digest based on the image tag if there are multiple
        # digests
        return image['RepoDigests'][0]

    if 'build' not in service.options:
        log.warn(
            "Compose needs to pull the image for '{s.name}' in order to create "
            "a bundle. This may result in a more recent image being used. "
            "It is recommended that you use an image tagged with a "
            "specific version to minimize the potential "
            "differences.".format(s=service))
        digest = service.pull()
    else:
        try:
            digest = service.push()
        except:
            log.error(
                "Failed to push image for service '{s.name}'. Please use an "
                "image tag that can be pushed to a Docker "
                "registry.".format(s=service))
            raise

    if not digest:
        raise ValueError("Failed to get digest for %s" % service.name)

    identifier = '{repo}@{digest}'.format(repo=repo, digest=digest)

    # Pull by digest so that image['RepoDigests'] is populated for next time
    # and we don't have to pull/push again
    service.client.pull(identifier)

    return identifier


def to_bundle(config, image_digests):
    config = denormalize_config(config)

    return {
        'version': VERSION,
        'services': {
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
            log.warn("Unsupported key '{}' in services.{} - ignoring".format(key, name))
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


# See https://github.com/docker/swarmkit/blob//agent/exec/container/container.go#L95
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
            log.warn(
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
