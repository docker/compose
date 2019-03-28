from __future__ import absolute_import
from __future__ import unicode_literals

from configparser import Error
from requests.exceptions import HTTPError
from twine.commands.upload import main as twine_upload
from twine.utils import get_config

from .utils import ScriptError


def pypi_upload(args):
    print('Uploading to PyPi')
    try:
        rel = args.release.replace('-rc', 'rc')
        twine_upload([
            'dist/docker_compose-{}*.whl'.format(rel),
            'dist/docker-compose-{}*.tar.gz'.format(rel)
        ])
    except HTTPError as e:
        if e.response.status_code == 400 and 'File already exists' in str(e):
            if not args.finalize_resume:
                raise ScriptError(
                    'Package already uploaded on PyPi.'
                )
            print('Skipping PyPi upload - package already uploaded')
        else:
            raise ScriptError('Unexpected HTTP error uploading package to PyPi: {}'.format(e))


def check_pypirc():
    try:
        config = get_config()
    except Error as e:
        raise ScriptError('Failed to parse .pypirc file: {}'.format(e))

    if config is None:
        raise ScriptError('Failed to parse .pypirc file')

    if 'pypi' not in config:
        raise ScriptError('Missing [pypi] section in .pypirc file')

    if not (config['pypi'].get('username') and config['pypi'].get('password')):
        raise ScriptError('Missing login/password pair for pypi repo')
