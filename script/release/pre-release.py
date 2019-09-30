from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import argparse
import os
import shutil
import sys

from release.const import NAME
from release.const import REPO_ROOT
from release.repository import Repository
from release.utils import ScriptError
from release.utils import update_init_py_version
from release.utils import update_run_sh_version
from release.utils import yesno


def create_initial_branch(repository, args):
    release_branch = repository.create_release_branch(args.release, args.base)
    if args.base and args.cherries:
        print('Detected patch version.')
        cherries = input('Indicate (space-separated) PR numbers to cherry-pick then press Enter:\n')
        repository.cherry_pick_prs(release_branch, cherries.split())

    return create_bump_commit(repository, release_branch)


def create_bump_commit(repository, release_branch):
    with release_branch.config_reader() as cfg:
        release = cfg.get('release')
    print('Updating version info in __init__.py and run.sh')
    update_run_sh_version(release)
    update_init_py_version(release)

    input('Please add the release notes to the CHANGELOG.md file, then press Enter to continue.')
    proceed = None
    while not proceed:
        print(repository.diff())
        proceed = yesno('Are these changes ok? y/N ', default=False)

    if repository.diff():
        repository.create_bump_commit(release_branch, release)
    repository.push_branch_to_remote(release_branch)


def check_pr_mergeable(pr_data):
    if pr_data.mergeable is False:
        # mergeable can also be null, in which case the warning would be a false positive.
        print(
            'WARNING!! PR #{} can not currently be merged. You will need to '
            'resolve the conflicts manually before finalizing the release.'.format(pr_data.number)
        )

    return pr_data.mergeable is True


def distclean():
    print('Running distclean...')
    dirs = [
        os.path.join(REPO_ROOT, 'build'), os.path.join(REPO_ROOT, 'dist'),
        os.path.join(REPO_ROOT, 'docker-compose.egg-info')
    ]
    files = []
    for base, dirnames, fnames in os.walk(REPO_ROOT):
        for fname in fnames:
            path = os.path.normpath(os.path.join(base, fname))
            if fname.endswith('.pyc'):
                files.append(path)
            elif fname.startswith('.coverage.'):
                files.append(path)
        for dirname in dirnames:
            path = os.path.normpath(os.path.join(base, dirname))
            if dirname == '__pycache__':
                dirs.append(path)
            elif dirname == '.coverage-binfiles':
                dirs.append(path)

    for file in files:
        os.unlink(file)

    for folder in dirs:
        shutil.rmtree(folder, ignore_errors=True)


def prepare(args):
    distclean()
    try:
        repository = Repository(REPO_ROOT, args.repo)
        create_initial_branch(repository, args)
        pr_data = repository.create_release_pull_request(args.release)
        check_pr_mergeable(pr_data)
    except ScriptError as e:
        print(e)
        return 1
    return 0


HELP = '''
    Prepare a new feature release (includes all changes currently in master)
        pre-release.sh 1.23.0
'''


def main():
    if 'GITHUB_TOKEN' not in os.environ:
        print('GITHUB_TOKEN environment variable must be set')
        return 1

    parser = argparse.ArgumentParser(
        description='Start a new feature release of docker/compose. This tool assumes that you have '
                    'obtained a Github API token and set the GITHUB_TOKEN environment variables '
                    'accordingly.',
        epilog=HELP, formatter_class=argparse.RawTextHelpFormatter)
    parser.add_argument('release', help='Release number, e.g. 1.9.0-rc1, 2.1.1')
    parser.add_argument(
        '--patch', '-p', dest='base',
        help='Which version is being patched by this release'
    )
    parser.add_argument(
        '--no-cherries', '-C', dest='cherries', action='store_false',
        help='If set, the program will not prompt the user for PR numbers to cherry-pick'
    )
    parser.add_argument(
        '--repo', '-r', dest='repo', default=NAME,
        help='Start a release for the given repo (default: {})'.format(NAME)
    )

    args = parser.parse_args()

    return prepare(args)


if __name__ == '__main__':
    sys.exit(main())
