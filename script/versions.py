#!/usr/bin/env python
"""
Query the github API for the git tags of a project, and return a list of
version tags for recent releases, or the default release.

The default release is the most recent non-RC version.

Recent is a list of unqiue major.minor versions, where each is the most
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
from __future__ import print_function

import argparse
import itertools
import operator
from collections import namedtuple

import requests


GITHUB_API = 'https://api.github.com/repos'


class Version(namedtuple('_Version', 'major minor patch rc')):

    @classmethod
    def parse(cls, version):
        version = version.lstrip('v')
        version, _, rc = version.partition('-')
        major, minor, patch = version.split('.', 3)
        return cls(int(major), int(minor), int(patch), rc)

    @property
    def major_minor(self):
        return self.major, self.minor

    @property
    def order(self):
        """Return a representation that allows this object to be sorted
        correctly with the default comparator.
        """
        # rc releases should appear before official releases
        rc = (0, self.rc) if self.rc else (1, )
        return (self.major, self.minor, self.patch) + rc

    def __str__(self):
        rc = '-{}'.format(self.rc) if self.rc else ''
        return '.'.join(map(str, self[:3])) + rc


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
    return [versions[index][0] for index in range(num)]


def get_default(versions):
    """Return a :class:`Version` for the latest non-rc version."""
    for version in versions:
        if not version.rc:
            return version


def get_github_releases(project):
    """Query the Github API for a list of version tags and return them in
    sorted order.

    See https://developer.github.com/v3/repos/#list-tags
    """
    url = '{}/{}/tags'.format(GITHUB_API, project)
    response = requests.get(url)
    response.raise_for_status()
    versions = [Version.parse(tag['name']) for tag in response.json()]
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
    versions = get_github_releases(args.project)

    if args.command == 'recent':
        print(' '.join(map(str, get_latest_versions(versions, args.num))))
    elif args.command == 'default':
        print(get_default(versions))
    else:
        raise ValueError("Unknown command {}".format(args.command))


if __name__ == "__main__":
    main()
