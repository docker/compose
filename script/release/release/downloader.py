from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import hashlib
import os

import requests

from .const import BINTRAY_ORG
from .const import NAME
from .const import REPO_ROOT
from .utils import branch_name


class BinaryDownloader(requests.Session):
    base_bintray_url = 'https://dl.bintray.com/{}'.format(BINTRAY_ORG)
    base_appveyor_url = 'https://ci.appveyor.com/api/projects/{}/artifacts/'.format(NAME)

    def __init__(self, destination, *args, **kwargs):
        super(BinaryDownloader, self).__init__(*args, **kwargs)
        self.destination = destination
        os.makedirs(self.destination, exist_ok=True)

    def download_from_bintray(self, repo_name, filename):
        print('Downloading {} from bintray'.format(filename))
        url = '{base}/{repo_name}/{filename}'.format(
            base=self.base_bintray_url, repo_name=repo_name, filename=filename
        )
        full_dest = os.path.join(REPO_ROOT, self.destination, filename)
        return self._download(url, full_dest)

    def download_from_appveyor(self, branch_name, filename):
        print('Downloading {} from appveyor'.format(filename))
        url = '{base}/dist%2F{filename}?branch={branch_name}'.format(
            base=self.base_appveyor_url, filename=filename, branch_name=branch_name
        )
        full_dest = os.path.join(REPO_ROOT, self.destination, filename)
        return self._download(url, full_dest)

    def _download(self, url, full_dest):
        m = hashlib.sha256()
        with open(full_dest, 'wb') as f:
            r = self.get(url, stream=True)
            for chunk in r.iter_content(chunk_size=1024 * 600, decode_unicode=False):
                print('.', end='', flush=True)
                m.update(chunk)
                f.write(chunk)

        print(' download complete')
        hex_digest = m.hexdigest()
        with open(full_dest + '.sha256', 'w') as f:
            f.write('{}  {}\n'.format(hex_digest, os.path.basename(full_dest)))
        return full_dest, hex_digest

    def download_all(self, version):
        files = {
            'docker-compose-Darwin-x86_64': None,
            'docker-compose-Linux-x86_64': None,
            'docker-compose-Windows-x86_64.exe': None,
        }

        for filename in files.keys():
            if 'Windows' in filename:
                files[filename] = self.download_from_appveyor(
                    branch_name(version), filename
                )
            else:
                files[filename] = self.download_from_bintray(
                    branch_name(version), filename
                )
        return files
