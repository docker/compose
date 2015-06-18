from __future__ import absolute_import
from textwrap import dedent


class UserError(Exception):
    def __init__(self, msg):
        self.msg = dedent(msg).strip()

    def __unicode__(self):
        return self.msg

    __str__ = __unicode__


class DockerNotFoundMac(UserError):
    def __init__(self):
        super(DockerNotFoundMac, self).__init__("""
        Couldn't connect to Docker daemon. You might need to install docker-osx:

        https://github.com/noplay/docker-osx
        """)


class DockerNotFoundUbuntu(UserError):
    def __init__(self):
        super(DockerNotFoundUbuntu, self).__init__("""
        Couldn't connect to Docker daemon. You might need to install Docker:

        http://docs.docker.io/en/latest/installation/ubuntulinux/
        """)


class DockerNotFoundGeneric(UserError):
    def __init__(self):
        super(DockerNotFoundGeneric, self).__init__("""
        Couldn't connect to Docker daemon. You might need to install Docker:

        http://docs.docker.io/en/latest/installation/
        """)


class ConnectionErrorBoot2Docker(UserError):
    def __init__(self):
        super(ConnectionErrorBoot2Docker, self).__init__("""
        Couldn't connect to Docker daemon - you might need to run `boot2docker up`.
        """)


class ConnectionErrorGeneric(UserError):
    def __init__(self, url):
        super(ConnectionErrorGeneric, self).__init__("""
        Couldn't connect to Docker daemon at %s - is it running?

        If it's at a non-standard location, specify the URL with the DOCKER_HOST environment variable.
        """ % url)


class ComposeFileNotFound(UserError):
    def __init__(self, supported_filenames):
        super(ComposeFileNotFound, self).__init__("""
        Can't find a suitable configuration file in this directory or any parent. Are you in the right directory?

        Supported filenames: %s
        """ % ", ".join(supported_filenames))
