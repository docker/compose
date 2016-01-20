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


log = logging.getLogger('migrate')


def migrate(content):
    data = ruamel.yaml.load(content, ruamel.yaml.RoundTripLoader)

    service_names = data.keys()
    for name, service in data.items():
        # remove links and external links
        service.pop('links', None)
        external_links = service.pop('external_links', None)
        if external_links:
            log.warn(
                "Service {name} has external_links: {ext}, which are no longer "
                "supported. See https://docs.docker.com/compose/networking/ "
                "for options on how to connect external containers to the "
                "compose network.".format(name=name, ext=external_links))

        # net is now networks
        if 'net' in service:
            service['networks'] = [service.pop('net')]

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

    data['services'] = {name: data.pop(name) for name in data.keys()}
    data['version'] = 2
    return data


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
    logging.basicConfig()

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
