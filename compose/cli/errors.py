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

        https://docs.docker.com/engine/installation/ubuntulinux/
        """)


class DockerNotFoundGeneric(UserError):
    def __init__(self):
        super(DockerNotFoundGeneric, self).__init__("""
        Couldn't connect to Docker daemon. You might need to install Docker:

        https://docs.docker.com/engine/installation/
        """)


class ConnectionErrorDockerMachine(UserError):
    def __init__(self):
        super(ConnectionErrorDockerMachine, self).__init__("""
        Couldn't connect to Docker daemon - you might need to run `docker-machine start default`.
        """)


class ConnectionErrorGeneric(UserError):
    def __init__(self, url):
        super(ConnectionErrorGeneric, self).__init__("""
        Couldn't connect to Docker daemon at %s - is it running?

        If it's at a non-standard location, specify the URL with the DOCKER_HOST environment variable.
        """ % url)
