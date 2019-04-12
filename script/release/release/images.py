from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import base64
import json
import os

import docker
from enum import Enum

from .const import NAME
from .const import REPO_ROOT
from .utils import ScriptError


class Platform(Enum):
    ALPINE = 'alpine'
    DEBIAN = 'debian'

    def __str__(self):
        return self.value


class ImageManager(object):
    def __init__(self, version, latest=False):
        self.built_tags = []
        self.docker_client = docker.APIClient(**docker.utils.kwargs_from_env())
        self.version = version
        self.latest = latest
        if 'HUB_CREDENTIALS' in os.environ:
            print('HUB_CREDENTIALS found in environment, issuing login')
            credentials = json.loads(base64.urlsafe_b64decode(os.environ['HUB_CREDENTIALS']))
            self.docker_client.login(
                username=credentials['Username'], password=credentials['Password']
            )

    def _tag(self, image, existing_tag, new_tag):
        existing_repo_tag = '{image}:{tag}'.format(image=image, tag=existing_tag)
        new_repo_tag = '{image}:{tag}'.format(image=image, tag=new_tag)
        self.docker_client.tag(existing_repo_tag, new_repo_tag)
        self.built_tags.append(new_repo_tag)

    def build_runtime_image(self, repository, platform):
        git_sha = repository.write_git_sha()
        compose_image_base_name = NAME
        print('Building {image} image ({platform} based)'.format(
            image=compose_image_base_name,
            platform=platform
        ))
        full_version = '{version}-{platform}'.format(version=self.version, platform=platform)
        build_tag = '{image_base_image}:{full_version}'.format(
            image_base_image=compose_image_base_name,
            full_version=full_version
        )
        logstream = self.docker_client.build(
            REPO_ROOT,
            tag=build_tag,
            buildargs={
                'BUILD_PLATFORM': platform.value,
                'GIT_COMMIT': git_sha,
            },
            decode=True
        )
        for chunk in logstream:
            if 'error' in chunk:
                raise ScriptError('Build error: {}'.format(chunk['error']))
            if 'stream' in chunk:
                print(chunk['stream'], end='')

        self.built_tags.append(build_tag)
        if platform == Platform.ALPINE:
            self._tag(compose_image_base_name, full_version, self.version)
        if self.latest:
            self._tag(compose_image_base_name, full_version, platform)
            if platform == Platform.ALPINE:
                self._tag(compose_image_base_name, full_version, 'latest')

    # Used for producing a test image for UCP
    def build_ucp_test_image(self, repository):
        print('Building test image (debian based for UCP e2e)')
        git_sha = repository.write_git_sha()
        compose_tests_image_base_name = NAME + '-tests'
        ucp_test_image_tag = '{image}:{tag}'.format(
            image=compose_tests_image_base_name,
            tag=self.version
        )
        logstream = self.docker_client.build(
            REPO_ROOT,
            tag=ucp_test_image_tag,
            target='build',
            buildargs={
                'BUILD_PLATFORM': Platform.DEBIAN.value,
                'GIT_COMMIT': git_sha,
            },
            decode=True
        )
        for chunk in logstream:
            if 'error' in chunk:
                raise ScriptError('Build error: {}'.format(chunk['error']))
            if 'stream' in chunk:
                print(chunk['stream'], end='')

        self.built_tags.append(ucp_test_image_tag)
        self._tag(compose_tests_image_base_name, self.version, 'latest')

    def build_images(self, repository):
        self.build_runtime_image(repository, Platform.ALPINE)
        self.build_runtime_image(repository, Platform.DEBIAN)
        self.build_ucp_test_image(repository)

    def check_images(self):
        for name in self.built_tags:
            try:
                self.docker_client.inspect_image(name)
            except docker.errors.ImageNotFound:
                print('Expected image {} was not found'.format(name))
                return False
        return True

    def push_images(self):
        for name in self.built_tags:
            print('Pushing {} to Docker Hub'.format(name))
            logstream = self.docker_client.push(name, stream=True, decode=True)
            for chunk in logstream:
                if 'status' in chunk:
                    print(chunk['status'])
                if 'error' in chunk:
                    raise ScriptError(
                        'Error pushing {name}: {err}'.format(name=name, err=chunk['error'])
                    )
