from __future__ import absolute_import
from __future__ import unicode_literals

import os
import re
import sys

import docker

import compose
from compose.const import IS_WINDOWS_PLATFORM


def __get_compose_version():
    return compose.__version__


def __get_docker_version():
    return ".".join([str(i) for i in docker.version_info])


def __get_platform():
    return sys.platform


def _get_uid():
    if IS_WINDOWS_PLATFORM:
        raise PosixOnlyHelperException(
                'get_user_id helper is only available'
                'on Posix operating systems')
    return os.getuid()


def _get_gid():
    if IS_WINDOWS_PLATFORM:
        raise PosixOnlyHelperException(
                'get_group_id helper is only available'
                'on Posix operating systems')
    return os.getgid()

func_map = {
    "get_user_id": _get_uid,
    "get_group_id": _get_gid,
    "get_compose_version": __get_compose_version,
    "get_docker_version": __get_docker_version,
    "get_host_platform": __get_platform
}

func_regexp = re.compile(r'@\{(.*?)\}')
inhibate_double_arobase = re.compile(r'@{2}{(.*?)}')


class InvalidHelperFunction(Exception):
    def __init__(self, string):
        self.string = string


class PosixOnlyHelperException(Exception):
    def __init__(self, string):
        self.string = string
