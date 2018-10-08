#!/usr/bin/env python
"""
Query the github API for the git tags of a project, and return a list of
version tags for recent releases, or the default release.

The default release is the most recent non-RC version.

Recent is a list of unique major.minor versions, where each is the most
recent version in the series.

For example, if the list of versions is:

    1.8.0-rc2
    1.8.0-rc1
    1.7.1
    1.7.0
    1.7.0-rc1
    1.6.2
    1.6.1

`default` would return `1.7.1` and
`recent -n 3` would return `1.8.0-rc2 1.7.1 1.6.2`
"""
from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import argparse
import itertools
import operator
import sys
from collections import namedtuple

import requests


GITHUB_API = 'https://api.github.com/repos'

STAGES = ['tp', 'beta', 'rc']


class Version(namedtuple('_Version', 'major minor patch stage edition')):

    @classmethod
    def parse(cls, version):
        edition = None
        version = version.lstrip('v')
        version, _, stage = version.partition('-')
        if stage:
            if not any(marker in stage for marker in STAGES):
                edition = stage
                stage = None
            elif '-' in stage:
                edition, stage = stage.split('-')
        major, minor, patch = version.split('.', 3)
        return cls(major, minor, patch, stage, edition)

    @property
    def major_minor(self):
        return self.major, self.minor

    @property
    def order(self):
        """Return a representation that allows this object to be sorted
        correctly with the default comparator.
        """
        # non-GA releases should appear before GA releases
        # Order: tp -> beta -> rc -> GA
        if self.stage:
            for st in STAGES:
                if st in self.stage:
                    stage = (STAGES.index(st), self.stage)
                    break
        else:
            stage = (len(STAGES),)

        return (int(self.major), int(self.minor), int(self.patch)) + stage

    def __str__(self):
        stage = '-{}'.format(self.stage) if self.stage else ''
        edition = '-{}'.format(self.edition) if self.edition else ''
        return '.'.join(map(str, self[:3])) + edition + stage


BLACKLIST = [  # List of versions known to be broken and should not be used
    Version.parse('18.03.0-ce-rc2'),
]


def group_versions(versions):
    """Group versions by `major.minor` releases.

    Example:

        >>> group_versions([
                Version(1, 0, 0),
                Version(2, 0, 0, 'rc1'),
                Version(2, 0, 0),
                Version(2, 1, 0),
            ])

        [
            [Version(1, 0, 0)],
            [Version(2, 0, 0), Version(2, 0, 0, 'rc1')],
            [Version(2, 1, 0)],
        ]
    """
    return list(
        list(releases)
        for _, releases
        in itertools.groupby(versions, operator.attrgetter('major_minor'))
    )


def get_latest_versions(versions, num=1):
    """Return a list of the most recent versions for each major.minor version
    group.
    """
    versions = group_versions(versions)
    num = min(len(versions), num)
    return [versions[index][0] for index in range(num)]


def get_default(versions):
    """Return a :class:`Version` for the latest GA version."""
    for version in versions:
        if not version.stage:
            return version


def get_versions(tags):
    for tag in tags:
        try:
            v = Version.parse(tag['name'])
            if v in BLACKLIST:
                continue
            yield v
        except ValueError:
            print("Skipping invalid tag: {name}".format(**tag), file=sys.stderr)


def get_github_releases(projects):
    """Query the Github API for a list of version tags and return them in
    sorted order.

    See https://developer.github.com/v3/repos/#list-tags
    """
    versions = []
    for project in projects:
        url = '{}/{}/tags'.format(GITHUB_API, project)
        response = requests.get(url)
        response.raise_for_status()
        versions.extend(get_versions(response.json()))
    return sorted(versions, reverse=True, key=operator.attrgetter('order'))


def parse_args(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('project', help="Github project name (ex: docker/docker)")
    parser.add_argument('command', choices=['recent', 'default'])
    parser.add_argument('-n', '--num', type=int, default=2,
                        help="Number of versions to return from `recent`")
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    versions = get_github_releases(args.project.split(','))

    if args.command == 'recent':
        print(' '.join(map(str, get_latest_versions(versions, args.num))))
    elif args.command == 'default':
        print(get_default(versions))
    else:
        raise ValueError("Unknown command {}".format(args.command))


if __name__ == "__main__":
    main()
