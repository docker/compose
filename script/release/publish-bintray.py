#!/usr/bin/env python
"""
Publish the binaries to a Bintray repository
"""
from __future__ import absolute_import
from __future__ import unicode_literals

import argparse
import os

from release.bintray import BintrayAPI
from release.const import BINTRAY_ORG


def publish_bintray(file, bintray_org, release_branch, platform):
    bintray_api = BintrayAPI(os.environ['BINTRAY_TOKEN'], os.environ['BINTRAY_USER'])
    if not bintray_api.repository_exists(bintray_org, release_branch):
        print('Creating data repository {} on bintray'.format(release_branch))
        bintray_api.create_repository(bintray_org, release_branch, 'generic')
    else:
        print('Bintray repository {} already exists. Skipping'.format(release_branch))
    print('Creating package {} on bintray'.format(platform))
    bintray_api.create_package(bintray_org, release_branch, platform)
    print('Creating version {} for {} on bintray'.format(release_branch, platform))
    bintray_api.create_version(bintray_org, release_branch, platform)
    print('Uploading file to bintray')
    bintray_api.upload_file(bintray_org, release_branch, platform, file)


def parse_args(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('-f', '--file', help="File to publish")
    parser.add_argument('--os', choices=["linux", "osx", "windows"],
                        help="Platform of the file to publish")
    parser.add_argument('-b', '--branch', help="The CI branch to publish")
    parser.add_argument(
        '--bintray-org', dest='bintray_org', metavar='ORG', default=BINTRAY_ORG,
        help='Organization name on bintray where the data repository will be created.'
    )

    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    # print("publish_bintray(%s, %s, %s, %s)" % (args.file, args.bintray_org, args.branch, args.os))
    publish_bintray(args.file, args.bintray_org, args.branch, args.os)


if __name__ == "__main__":
    main()
