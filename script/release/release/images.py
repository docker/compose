from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import base64
import json
import os
import shutil

import docker

from .const import REPO_ROOT
from .utils import ScriptError


class ImageManager(object):
    def __init__(self, version):
        self.docker_client = docker.APIClient(**docker.utils.kwargs_from_env())
        self.version = version
        if 'HUB_CREDENTIALS' in os.environ:
            print('HUB_CREDENTIALS found in environment, issuing login')
            credentials = json.loads(base64.urlsafe_b64decode(os.environ['HUB_CREDENTIALS']))
            self.docker_client.login(
                username=credentials['Username'], password=credentials['Password']
            )

    def build_images(self, repository, files):
        print("Building release images...")
        repository.write_git_sha()
        distdir = os.path.join(REPO_ROOT, 'dist')
        os.makedirs(distdir, exist_ok=True)
        shutil.copy(files['docker-compose-Linux-x86_64'][0], distdir)
        os.chmod(os.path.join(distdir, 'docker-compose-Linux-x86_64'), 0o755)
        print('Building docker/compose image')
        logstream = self.docker_client.build(
            REPO_ROOT, tag='docker/compose:{}'.format(self.version), dockerfile='Dockerfile.run',
            decode=True
        )
        for chunk in logstream:
            if 'error' in chunk:
                raise ScriptError('Build error: {}'.format(chunk['error']))
            if 'stream' in chunk:
                print(chunk['stream'], end='')

        print('Building test image (for UCP e2e)')
        logstream = self.docker_client.build(
            REPO_ROOT, tag='docker-compose-tests:tmp', decode=True
        )
        for chunk in logstream:
            if 'error' in chunk:
                raise ScriptError('Build error: {}'.format(chunk['error']))
            if 'stream' in chunk:
                print(chunk['stream'], end='')

        container = self.docker_client.create_container(
            'docker-compose-tests:tmp', entrypoint='tox'
        )
        self.docker_client.commit(container, 'docker/compose-tests', 'latest')
        self.docker_client.tag(
            'docker/compose-tests:latest', 'docker/compose-tests:{}'.format(self.version)
        )
        self.docker_client.remove_container(container, force=True)
        self.docker_client.remove_image('docker-compose-tests:tmp', force=True)

    @property
    def image_names(self):
        return [
            'docker/compose-tests:latest',
            'docker/compose-tests:{}'.format(self.version),
            'docker/compose:{}'.format(self.version)
        ]

    def check_images(self):
        for name in self.image_names:
            try:
                self.docker_client.inspect_image(name)
            except docker.errors.ImageNotFound:
                print('Expected image {} was not found'.format(name))
                return False
        return True

    def push_images(self):
        for name in self.image_names:
            print('Pushing {} to Docker Hub'.format(name))
            logstream = self.docker_client.push(name, stream=True, decode=True)
            for chunk in logstream:
                if 'status' in chunk:
                    print(chunk['status'])
                if 'error' in chunk:
                    raise ScriptError(
                        'Error pushing {name}: {err}'.format(name=name, err=chunk['error'])
                    )
