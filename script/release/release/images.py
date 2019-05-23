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
from .utils import yesno
from script.release.release.const import COMPOSE_TESTS_IMAGE_BASE_NAME


class Platform(Enum):
    ALPINE = 'alpine'
    DEBIAN = 'debian'

    def __str__(self):
        return self.value


# Checks if this version respects the GA version format ('x.y.z') and not an RC
def is_tag_latest(version):
    ga_version = all(n.isdigit() for n in version.split('.')) and version.count('.') == 2
    return ga_version and yesno('Should this release be tagged as \"latest\"? [Y/n]: ', default=True)


class ImageManager(object):
    def __init__(self, version, latest=False):
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

    def get_full_version(self, platform=None):
        return self.version + '-' + platform.__str__() if platform else self.version

    def get_runtime_image_tag(self, tag):
        return '{image_base_image}:{tag}'.format(
            image_base_image=NAME,
            tag=self.get_full_version(tag)
        )

    def build_runtime_image(self, repository, platform):
        git_sha = repository.write_git_sha()
        compose_image_base_name = NAME
        print('Building {image} image ({platform} based)'.format(
            image=compose_image_base_name,
            platform=platform
        ))
        full_version = self.get_full_version(platform)
        build_tag = self.get_runtime_image_tag(platform)
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

        if platform == Platform.ALPINE:
            self._tag(compose_image_base_name, full_version, self.version)
        if self.latest:
            self._tag(compose_image_base_name, full_version, platform)
            if platform == Platform.ALPINE:
                self._tag(compose_image_base_name, full_version, 'latest')

    def get_ucp_test_image_tag(self, tag=None):
        return '{image}:{tag}'.format(
            image=COMPOSE_TESTS_IMAGE_BASE_NAME,
            tag=tag or self.version
        )

    # Used for producing a test image for UCP
    def build_ucp_test_image(self, repository):
        print('Building test image (debian based for UCP e2e)')
        git_sha = repository.write_git_sha()
        ucp_test_image_tag = self.get_ucp_test_image_tag()
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

        self._tag(COMPOSE_TESTS_IMAGE_BASE_NAME, self.version, 'latest')

    def build_images(self, repository):
        self.build_runtime_image(repository, Platform.ALPINE)
        self.build_runtime_image(repository, Platform.DEBIAN)
        self.build_ucp_test_image(repository)

    def check_images(self):
        for name in self.get_images_to_push():
            try:
                self.docker_client.inspect_image(name)
            except docker.errors.ImageNotFound:
                print('Expected image {} was not found'.format(name))
                return False
        return True

    def get_images_to_push(self):
        tags_to_push = {
            "{}:{}".format(NAME, self.version),
            self.get_runtime_image_tag(Platform.ALPINE),
            self.get_runtime_image_tag(Platform.DEBIAN),
            self.get_ucp_test_image_tag(),
            self.get_ucp_test_image_tag('latest'),
        }
        if is_tag_latest(self.version):
            tags_to_push.add("{}:latest".format(NAME))
        return tags_to_push

    def push_images(self):
        tags_to_push = self.get_images_to_push()
        print('Build tags to push {}'.format(tags_to_push))
        for name in tags_to_push:
            print('Pushing {} to Docker Hub'.format(name))
            logstream = self.docker_client.push(name, stream=True, decode=True)
            for chunk in logstream:
                if 'status' in chunk:
                    print(chunk['status'])
                if 'error' in chunk:
                    raise ScriptError(
                        'Error pushing {name}: {err}'.format(name=name, err=chunk['error'])
                    )
