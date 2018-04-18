from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import argparse
import os
import sys
import time

from jinja2 import Template
from release.bintray import BintrayAPI
from release.const import BINTRAY_ORG
from release.const import NAME
from release.const import REPO_ROOT
from release.downloader import BinaryDownloader
from release.repository import get_contributors
from release.repository import Repository
from release.repository import upload_assets
from release.utils import branch_name
from release.utils import compatibility_matrix
from release.utils import read_release_notes_from_changelog
from release.utils import ScriptError
from release.utils import update_init_py_version
from release.utils import update_run_sh_version


def create_initial_branch(repository, release, base, bintray_user):
    release_branch = repository.create_release_branch(release, base)
    print('Updating version info in __init__.py and run.sh')
    update_run_sh_version(release)
    update_init_py_version(release)

    input('Please add the release notes to the CHANGELOG.md file, then press Enter to continue.')
    proceed = ''
    while proceed.lower() != 'y':
        print(repository.diff())
        proceed = input('Are these changes ok? y/N ')

    repository.create_bump_commit(release_branch, release)
    repository.push_branch_to_remote(release_branch)

    bintray_api = BintrayAPI(os.environ['BINTRAY_TOKEN'], bintray_user)
    print('Creating data repository {} on bintray'.format(release_branch.name))
    bintray_api.create_repository(BINTRAY_ORG, release_branch.name, 'generic')


def monitor_pr_status(pr_data):
    print('Waiting for CI to complete...')
    last_commit = pr_data.get_commits().reversed[0]
    while True:
        status = last_commit.get_combined_status()
        if status.state == 'pending':
            summary = {
                'pending': 0,
                'success': 0,
                'failure': 0,
            }
            for detail in status.statuses:
                summary[detail.state] += 1
            print('{pending} pending, {success} successes, {failure} failures'.format(**summary))
            if status.total_count == 0:
                # Mostly for testing purposes against repos with no CI setup
                return True
            time.sleep(30)
        elif status.state == 'success':
            print('{} successes: all clear!'.format(status.total_count))
            return True
        else:
            raise ScriptError('CI failure detected')


def create_release_draft(repository, version, pr_data, files):
    print('Creating Github release draft')
    with open(os.path.join(os.path.dirname(__file__), 'release.md.tmpl'), 'r') as f:
        template = Template(f.read())
    print('Rendering release notes based on template')
    release_notes = template.render(
        version=version,
        compat_matrix=compatibility_matrix(),
        integrity=files,
        contributors=get_contributors(pr_data),
        changelog=read_release_notes_from_changelog(),
    )
    gh_release = repository.create_release(
        version, release_notes, draft=True, prerelease='-rc' in version,
        target_commitish='release'
    )
    print('Release draft initialized')
    return gh_release


def resume(args):
    raise NotImplementedError()
    try:
        repository = Repository(REPO_ROOT, args.repo or NAME)
        br_name = branch_name(args.release)
        if not repository.branch_exists(br_name):
            raise ScriptError('No local branch exists for this release.')
        # release_branch = repository.checkout_branch(br_name)
    except ScriptError as e:
        print(e)
        return 1
    return 0


def cancel(args):
    try:
        repository = Repository(REPO_ROOT, args.repo or NAME)
        repository.close_release_pr(args.release)
        repository.remove_release(args.release)
        repository.remove_bump_branch(args.release)
        # TODO: uncomment after testing is complete
        # bintray_api = BintrayAPI(os.environ['BINTRAY_TOKEN'], args.bintray_user)
        # print('Removing Bintray data repository for {}'.format(args.release))
        # bintray_api.delete_repository(BINTRAY_ORG, branch_name(args.release))
    except ScriptError as e:
        print(e)
        return 1
    print('Release cancellation complete.')
    return 0


def start(args):
    try:
        repository = Repository(REPO_ROOT, args.repo or NAME)
        create_initial_branch(repository, args.release, args.base, args.bintray_user)
        pr_data = repository.create_release_pull_request(args.release)
        monitor_pr_status(pr_data)
        downloader = BinaryDownloader(args.destination)
        files = downloader.download_all(args.release)
        gh_release = create_release_draft(repository, args.release, pr_data, files)
        upload_assets(gh_release, files)
    except ScriptError as e:
        print(e)
        return 1

    return 0


def main():
    if 'GITHUB_TOKEN' not in os.environ:
        print('GITHUB_TOKEN environment variable must be set')
        return 1

    if 'BINTRAY_TOKEN' not in os.environ:
        print('BINTRAY_TOKEN environment variable must be set')
        return 1

    parser = argparse.ArgumentParser(
        description='Orchestrate a new release of docker/compose. This tool assumes that you have'
                    'obtained a Github API token and Bintray API key and set the GITHUB_TOKEN and'
                    'BINTRAY_TOKEN environment variables accordingly.',
        epilog='''Example uses:
    * Start a new feature release (includes all changes currently in master)
        release.py -b user start 1.23.0
    * Start a new patch release
        release.py -b user --patch 1.21.0 start 1.21.1
    * Cancel / rollback an existing release draft
        release.py -b user cancel 1.23.0
    * Restart a previously aborted patch release
        release.py -b user -p 1.21.0 resume 1.21.1
    ''', formatter_class=argparse.RawTextHelpFormatter)
    parser.add_argument(
        'action', choices=['start', 'resume', 'cancel'],
        help='The action to be performed for this release'
    )
    parser.add_argument('release', help='Release number, e.g. 1.9.0-rc1, 2.1.1')
    parser.add_argument(
        '--patch', '-p', dest='base',
        help='Which version is being patched by this release'
    )
    parser.add_argument(
        '--repo', '-r', dest='repo',
        help='Start a release for the given repo (default: {})'.format(NAME)
    )
    parser.add_argument(
        '-b', dest='bintray_user', required=True, metavar='USER',
        help='Username associated with the Bintray API key'
    )
    parser.add_argument(
        '--destination', '-o', metavar='DIR', default='binaries',
        help='Directory where release binaries will be downloaded relative to the project root'
    )
    args = parser.parse_args()

    if args.action == 'start':
        return start(args)
    elif args.action == 'resume':
        return resume(args)
    elif args.action == 'cancel':
        return cancel(args)
    print('Unexpected action "{}"'.format(args.action), file=sys.stderr)
    return 1


if __name__ == '__main__':
    sys.exit(main())
