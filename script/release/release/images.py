from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import os
import shutil

import docker

from .const import REPO_ROOT
from .utils import ScriptError


class ImageManager(object):
    def __init__(self, version):
        self.docker_client = docker.APIClient(**docker.utils.kwargs_from_env())
        self.version = version

    def build_images(self, repository, files):
        print("Building release images...")
        repository.write_git_sha()
        docker_client = docker.APIClient(**docker.utils.kwargs_from_env())
        distdir = os.path.join(REPO_ROOT, 'dist')
        os.makedirs(distdir, exist_ok=True)
        shutil.copy(files['docker-compose-Linux-x86_64'][0], distdir)
        print('Building docker/compose image')
        logstream = docker_client.build(
            REPO_ROOT, tag='docker/compose:{}'.format(self.version), dockerfile='Dockerfile.run',
            decode=True
        )
        for chunk in logstream:
            if 'error' in chunk:
                raise ScriptError('Build error: {}'.format(chunk['error']))
            if 'stream' in chunk:
                print(chunk['stream'], end='')

        print('Building test image (for UCP e2e)')
        logstream = docker_client.build(
            REPO_ROOT, tag='docker-compose-tests:tmp', decode=True
        )
        for chunk in logstream:
            if 'error' in chunk:
                raise ScriptError('Build error: {}'.format(chunk['error']))
            if 'stream' in chunk:
                print(chunk['stream'], end='')

        container = docker_client.create_container(
            'docker-compose-tests:tmp', entrypoint='tox'
        )
        docker_client.commit(container, 'docker/compose-tests:latest')
        docker_client.tag('docker/compose-tests:latest', 'docker/compose-tests:{}'.format(self.version))
        docker_client.remove_container(container, force=True)
        docker_client.remove_image('docker-compose-tests:tmp', force=True)

    @property
    def image_names(self):
        return [
            'docker/compose-tests:latest',
            'docker/compose-tests:{}'.format(self.version),
            'docker/compose:{}'.format(self.version)
        ]

    def check_images(self, version):
        docker_client = docker.APIClient(**docker.utils.kwargs_from_env())

        for name in self.image_names:
            try:
                docker_client.inspect_image(name)
            except docker.errors.ImageNotFound:
                print('Expected image {} was not found'.format(name))
                return False
        return True

    def push_images(self):
        docker_client = docker.APIClient(**docker.utils.kwargs_from_env())

        for name in self.image_names:
            print('Pushing {} to Docker Hub'.format(name))
            logstream = docker_client.push(name, stream=True, decode=True)
            for chunk in logstream:
                if 'status' in chunk:
                    print(chunk['status'])
