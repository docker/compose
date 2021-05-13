#!/usr/bin/env python3
import re

import click
from git import Repo
from utils import update_init_py_version
from utils import update_run_sh_version
from utils import yesno

VALID_VERSION_PATTERN = re.compile(r"^\d+\.\d+\.\d+(-rc\d+)?$")


class Version(str):
    def matching_groups(self):
        match = VALID_VERSION_PATTERN.match(self)
        if not match:
            return False

        return match.groups()

    def is_ga_version(self):
        groups = self.matching_groups()
        if not groups:
            return False

        rc_suffix = groups[1]
        return not rc_suffix

    def validate(self):
        return len(self.matching_groups()) > 0

    def branch_name(self):
        if not self.validate():
            return None

        rc_part = self.matching_groups()[0]
        ver = self
        if rc_part:
            ver = ver[:-len(rc_part)]

        tokens = ver.split(".")
        tokens[-1] = 'x'

        return ".".join(tokens)


def create_bump_commit(repository, version):
    print('Creating bump commit...')
    repository.commit('-a', '-s', '-m "Bump {}"'.format(version), '--no-verify')


def validate_environment(version, repository):
    if not version.validate():
        print('Version "{}" has an invalid format. This should follow D+.D+.D+(-rcD+). '
              'Like: 1.26.0 or 1.26.0-rc1'.format(version))
        return False

    expected_branch = version.branch_name()
    if str(repository.active_branch) != expected_branch:
        print('Cannot tag in this branch with version "{}". '
              'Please checkout "{}" to tag'.format(version, version.branch_name()))
        return False
    return True


@click.group()
def cli():
    pass


@cli.command()
@click.argument('version')
def tag(version):
    """
    Updates the version related files and tag
    """
    repo = Repo(".")
    version = Version(version)
    if not validate_environment(version, repo):
        return

    update_init_py_version(version)
    update_run_sh_version(version)

    input('Please add the release notes to the CHANGELOG.md file, then press Enter to continue.')
    proceed = False
    while not proceed:
        print(repo.git.diff())
        proceed = yesno('Are these changes ok? y/N ', default=False)

    if repo.git.diff():
        create_bump_commit(repo.git, version)
    else:
        print('No changes to commit. Exiting...')
        return

    repo.create_tag(version)

    print('Please, check the changes. If everything is OK, you just need to push with:\n'
          '$ git push --tags upstream {}'.format(version.branch_name()))


@cli.command()
@click.argument('version')
def push_latest(version):
    """
    TODO Pushes the latest tag pointing to a certain GA version
    """
    raise NotImplementedError


@cli.command()
@click.argument('version')
def ghtemplate(version):
    """
    TODO Generates the github release page content
    """
    version = Version(version)
    raise NotImplementedError


if __name__ == '__main__':
    cli()
