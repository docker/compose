#!/usr/bin/env python
"""
Publish the python packages in the dist directory to PyPI
"""
from __future__ import absolute_import
from __future__ import unicode_literals

import argparse

from requests.exceptions import HTTPError
from twine.commands.upload import main as twine_upload


class ScriptError(Exception):
    pass


def pypi_upload(release):
    print('Uploading to PyPi')
    try:
        rel = release.replace('-rc', 'rc')
        twine_upload([
            'dist/docker_compose-{}*.whl'.format(rel),
            'dist/docker-compose-{}*.tar.gz'.format(rel)
        ])
    except HTTPError as e:
        if e.response.status_code == 400 and 'File already exists' in str(e):
            raise ScriptError('Package already uploaded on PyPi.')
        else:
            raise ScriptError('Unexpected HTTP error uploading package to PyPi: {}'.format(e))


def parse_args(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--release', help='Release number, e.g. 1.9.0-rc1, 2.1.1')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    pypi_upload(args.release)


if __name__ == "__main__":
    main()
