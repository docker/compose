#!/usr/bin/env python
"""
Migrate a Compose file from the V1 format in Compose 1.5 to the V2 format
supported by Compose 1.6+
"""
from __future__ import absolute_import
from __future__ import unicode_literals

import argparse
import logging
import sys

import ruamel.yaml

from compose.config.types import VolumeSpec


log = logging.getLogger('migrate')


def migrate(content):
    data = ruamel.yaml.load(content, ruamel.yaml.RoundTripLoader)

    service_names = data.keys()

    for name, service in data.items():
        links = service.get('links')
        if links:
            example_service = links[0].partition(':')[0]
            log.warn(
                "Service {name} has links, which no longer create environment "
                "variables such as {example_service_upper}_PORT. "
                "If you are using those in your application code, you should "
                "instead connect directly to the hostname, e.g. "
                "'{example_service}'."
                .format(name=name, example_service=example_service,
                        example_service_upper=example_service.upper()))

        external_links = service.get('external_links')
        if external_links:
            log.warn(
                "Service {name} has external_links: {ext}, which now work "
                "slightly differently. In particular, two containers must be "
                "connected to at least one network in common in order to "
                "communicate, even if explicitly linked together.\n\n"
                "Either connect the external container to your app's default "
                "network, or connect both the external container and your "
                "service's containers to a pre-existing network. See "
                "https://docs.docker.com/compose/networking/ "
                "for more on how to do this."
                .format(name=name, ext=external_links))

        # net is now network_mode
        if 'net' in service:
            network_mode = service.pop('net')

            # "container:<service name>" is now "service:<service name>"
            if network_mode.startswith('container:'):
                name = network_mode.partition(':')[2]
                if name in service_names:
                    network_mode = 'service:{}'.format(name)

            service['network_mode'] = network_mode

        # create build section
        if 'dockerfile' in service:
            service['build'] = {
                'context': service.pop('build'),
                'dockerfile': service.pop('dockerfile'),
            }

        # create logging section
        if 'log_driver' in service:
            service['logging'] = {'driver': service.pop('log_driver')}
            if 'log_opt' in service:
                service['logging']['options'] = service.pop('log_opt')

        # volumes_from prefix with 'container:'
        for idx, volume_from in enumerate(service.get('volumes_from', [])):
            if volume_from.split(':', 1)[0] not in service_names:
                service['volumes_from'][idx] = 'container:%s' % volume_from

    services = {name: data.pop(name) for name in data.keys()}

    data['version'] = 2
    data['services'] = services
    create_volumes_section(data)

    return data


def create_volumes_section(data):
    named_volumes = get_named_volumes(data['services'])
    if named_volumes:
        log.warn(
            "Named volumes ({names}) must be explicitly declared. Creating a "
            "'volumes' section with declarations.\n\n"
            "For backwards-compatibility, they've been declared as external. "
            "If you don't mind the volume names being prefixed with the "
            "project name, you can remove the 'external' option from each one."
            .format(names=', '.join(list(named_volumes))))

        data['volumes'] = named_volumes


def get_named_volumes(services):
    volume_specs = [
        VolumeSpec.parse(volume)
        for service in services.values()
        for volume in service.get('volumes', [])
    ]
    names = {
        spec.external
        for spec in volume_specs
        if spec.is_named_volume
    }
    return {name: {'external': True} for name in names}


def write(stream, new_format, indent, width):
    ruamel.yaml.dump(
        new_format,
        stream,
        Dumper=ruamel.yaml.RoundTripDumper,
        indent=indent,
        width=width)


def parse_opts(args):
    parser = argparse.ArgumentParser()
    parser.add_argument("filename", help="Compose file filename.")
    parser.add_argument("-i", "--in-place", action='store_true')
    parser.add_argument(
        "--indent", type=int, default=2,
        help="Number of spaces used to indent the output yaml.")
    parser.add_argument(
        "--width", type=int, default=80,
        help="Number of spaces used as the output width.")
    return parser.parse_args()


def main(args):
    logging.basicConfig(format='\033[33m%(levelname)s:\033[37m %(message)s\n')

    opts = parse_opts(args)

    with open(opts.filename, 'r') as fh:
        new_format = migrate(fh.read())

    if opts.in_place:
        output = open(opts.filename, 'w')
    else:
        output = sys.stdout
    write(output, new_format, opts.indent, opts.width)


if __name__ == "__main__":
    main(sys.argv)
